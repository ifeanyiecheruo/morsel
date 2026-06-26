package lint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ifeanyiecheruo/morsel/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Severity indicates whether a diagnostic represents an error or a warning.
type Severity int

const (
	Error Severity = iota
	Warning
)

// Diagnostic is a lint finding associated with a single file.
type Diagnostic struct {
	File     string
	Message  string
	Severity Severity
}

// File is a morsel.json file represented by its path and raw JSON content.
type File struct {
	Path    string
	Content []byte
}

// FixedFile holds the corrected content for a file that was reformatted by Fix.
type FixedFile struct {
	Path    string
	Content []byte
}

// Linter validates morsel.json files against the embedded schema and semantic rules.
type Linter struct {
	schema *jsonschema.Schema
}

// New creates a Linter with the embedded morsel.schema.json compiled.
func New() (*Linter, error) {
	var schemaDoc any
	if err := json.Unmarshal(schemas.MorselSchemaJSON, &schemaDoc); err != nil {
		return nil, fmt.Errorf("parse embedded schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("morsel.schema.json", schemaDoc); err != nil {
		return nil, fmt.Errorf("register schema: %w", err)
	}
	schema, err := compiler.Compile("morsel.schema.json")
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return &Linter{schema: schema}, nil
}

// Lint validates the given files and returns all diagnostics, including cross-file checks.
func (l *Linter) Lint(files []File) []Diagnostic {
	var diags []Diagnostic
	// name → file paths; used to detect duplicates across the set.
	names := make(map[string][]string)

	for _, file := range files {
		fileDiags, mf := l.lintFile(file)
		diags = append(diags, fileDiags...)
		if mf != nil && mf.Name != "" {
			names[mf.Name] = append(names[mf.Name], file.Path)
		}
	}

	for name, paths := range names {
		if len(paths) > 1 {
			joined := strings.Join(paths, ", ")
			for _, path := range paths {
				diags = append(diags, Diagnostic{
					File:     path,
					Message:  fmt.Sprintf("name %q is declared in multiple files: %s", name, joined),
					Severity: Error,
				})
			}
		}
	}

	return diags
}

// Fix returns canonical JSON for files whose formatting is non-standard.
// Only files that actually changed are returned. Files with unknown fields or
// parse errors are skipped silently — they need schema fixes before formatting.
func (l *Linter) Fix(files []File) ([]FixedFile, error) {
	var fixed []FixedFile
	for _, file := range files {
		corrected, changed, err := fixFile(file)
		if err != nil || !changed {
			continue
		}
		fixed = append(fixed, FixedFile{Path: file.Path, Content: corrected})
	}
	return fixed, nil
}

// --- private types ---

// morselFile mirrors the JSON structure. Field order here defines the canonical
// output order produced by Fix.
type morselFile struct {
	Schema      string       `json:"$schema,omitempty"`
	Name        string       `json:"name,omitempty"`
	Type        string       `json:"type,omitempty"`
	Dockerfile  string       `json:"dockerfile,omitempty"`
	Private     *bool        `json:"private,omitempty"`
	Tier        string       `json:"tier,omitempty"`
	IdleAfter   string       `json:"idle_after,omitempty"`
	HealthCheck *healthCheck `json:"health_check,omitempty"`
	Schedule    string       `json:"schedule,omitempty"`
	Timeout     string       `json:"timeout,omitempty"`
	Retries     *int         `json:"retries,omitempty"`
	Persistence *persistence `json:"persistence,omitempty"`
}

type healthCheck struct {
	Path    string `json:"path,omitempty"`
	Timeout string `json:"timeout,omitempty"`
}

type persistence struct {
	Database *persistenceResource `json:"database,omitempty"`
	Storage  *persistenceResource `json:"storage,omitempty"`
	Queues   *persistenceResource `json:"queues,omitempty"`
}

type persistenceResource struct {
	Permanent *bool `json:"permanent,omitempty"`
}

// --- private functions ---

func (l *Linter) lintFile(file File) ([]Diagnostic, *morselFile) {
	var diags []Diagnostic

	var raw any
	if err := json.Unmarshal(file.Content, &raw); err != nil {
		return []Diagnostic{{
			File:     file.Path,
			Message:  "invalid JSON: " + err.Error(),
			Severity: Error,
		}}, nil
	}

	if err := l.schema.Validate(raw); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			for _, msg := range leafErrors(ve) {
				diags = append(diags, Diagnostic{File: file.Path, Message: msg, Severity: Error})
			}
		} else {
			diags = append(diags, Diagnostic{File: file.Path, Message: err.Error(), Severity: Error})
		}
	}

	var mf morselFile
	if err := json.Unmarshal(file.Content, &mf); err != nil {
		return diags, nil
	}

	diags = append(diags, semanticDiags(file.Path, &mf)...)
	return diags, &mf
}

func semanticDiags(path string, mf *morselFile) []Diagnostic {
	var diags []Diagnostic
	warn := func(msg string) {
		diags = append(diags, Diagnostic{File: path, Message: msg, Severity: Warning})
	}

	switch mf.Type {
	case "worker":
		if mf.Private != nil {
			warn(`private is not applicable to type "worker" and will be ignored`)
		}
		if mf.HealthCheck != nil && mf.HealthCheck.Path != "" {
			warn(`health_check.path is not applicable to type "worker" and will be ignored`)
		}
		if mf.IdleAfter != "" {
			warn(`idle_after is not applicable to type "worker" and will be ignored`)
		}
		if mf.Schedule != "" {
			warn(`schedule is not applicable to type "worker" and will be ignored`)
		}
		if mf.Timeout != "" {
			warn(`timeout is not applicable to type "worker" and will be ignored`)
		}
		if mf.Retries != nil {
			warn(`retries is not applicable to type "worker" and will be ignored`)
		}
	case "cronjob":
		if mf.Private != nil {
			warn(`private is not applicable to type "cronjob" and will be ignored`)
		}
		if mf.HealthCheck != nil && mf.HealthCheck.Path != "" {
			warn(`health_check.path is not applicable to type "cronjob" and will be ignored`)
		}
		if mf.IdleAfter != "" {
			warn(`idle_after is not applicable to type "cronjob" and will be ignored`)
		}
	case "http":
		if mf.Schedule != "" {
			warn(`schedule is not applicable to type "http" and will be ignored`)
		}
		if mf.Timeout != "" {
			warn(`timeout is not applicable to type "http" and will be ignored`)
		}
		if mf.Retries != nil {
			warn(`retries is not applicable to type "http" and will be ignored`)
		}
	}

	if mf.Persistence != nil {
		if mf.Persistence.Database != nil && mf.Persistence.Database.Permanent != nil && *mf.Persistence.Database.Permanent {
			warn("persistence.database.permanent is true — removing this field will schedule deletion after a 30-day grace period")
		}
		if mf.Persistence.Storage != nil && mf.Persistence.Storage.Permanent != nil && *mf.Persistence.Storage.Permanent {
			warn("persistence.storage.permanent is true — removing this field will schedule deletion after a 30-day grace period")
		}
		if mf.Persistence.Queues != nil && mf.Persistence.Queues.Permanent != nil && *mf.Persistence.Queues.Permanent {
			warn("persistence.queues.permanent is true — removing this field will schedule deletion after a 30-day grace period")
		}
	}

	return diags
}

// fixFile returns the canonically formatted JSON for the file. Returns changed=false
// when the file is already canonical. Returns an error when the file has unknown
// fields or cannot be parsed — callers skip such files rather than corrupting them.
func fixFile(file File) ([]byte, bool, error) {
	dec := json.NewDecoder(bytes.NewReader(file.Content))
	dec.DisallowUnknownFields()
	var mf morselFile
	if err := dec.Decode(&mf); err != nil {
		return nil, false, err
	}
	if mf.Schema == "" {
		mf.Schema = "https://raw.githubusercontent.com/ifeanyiecheruo/morsel/main/schemas/morsel.schema.json"
	}
	corrected, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return nil, false, err
	}
	corrected = append(corrected, '\n')
	if bytes.Equal(corrected, file.Content) {
		return nil, false, nil
	}
	return corrected, true, nil
}

// leafErrors collects the deepest (leaf) validation error messages. Using leaf
// errors avoids redundant parent messages like "if-then not satisfied" when the
// meaningful detail is in the nested cause.
func leafErrors(ve *jsonschema.ValidationError) []string {
	if len(ve.Causes) == 0 {
		return []string{ve.Error()}
	}
	var msgs []string
	for _, cause := range ve.Causes {
		msgs = append(msgs, leafErrors(cause)...)
	}
	return msgs
}
