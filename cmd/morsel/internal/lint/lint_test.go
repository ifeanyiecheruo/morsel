package lint_test

import (
	"testing"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/lint"
)

// helpers

func mustNew(t *testing.T) *lint.Linter {
	t.Helper()
	l, err := lint.New()
	if err != nil {
		t.Fatalf("lint.New: %v", err)
	}
	return l
}

func file(path, content string) lint.File {
	return lint.File{Path: path, Content: []byte(content)}
}

func countBySeverity(diags []lint.Diagnostic, sev lint.Severity) int {
	count := 0
	for _, d := range diags {
		if d.Severity == sev {
			count++
		}
	}
	return count
}

func hasMessage(diags []lint.Diagnostic, substr string) bool {
	for _, d := range diags {
		if contains(d.Message, substr) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := range len(s) - len(substr) + 1 {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// --- New ---

func TestNewSucceeds(t *testing.T) {
	mustNew(t)
}

// --- Lint: valid files ---

func TestLintValidHTTPFile(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/api.morsel.json", `{"type":"http","dockerfile":"Dockerfile","port":8080}`),
	})
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics, got %d: %v", len(diags), diags)
	}
}

func TestLintValidWorkerFile(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/worker.morsel.json", `{"type":"worker","dockerfile":"worker/Dockerfile"}`),
	})
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics, got %d: %v", len(diags), diags)
	}
}

func TestLintValidCronjobFile(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/cron.morsel.json", `{"type":"cronjob","dockerfile":"Dockerfile","schedule":"0 2 * * *","timeout":"30m"}`),
	})
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics, got %d: %v", len(diags), diags)
	}
}

func TestLintValidHTTPFileAllFields(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/api.morsel.json", `{
			"name":"api","type":"http","dockerfile":"api/Dockerfile",
			"port":8080,"private":false,"tier":"small","idle_after":"24h",
			"health_check":{"path":"/healthz","timeout":"5m"},
			"persistence":{"database":{"permanent":true},"storage":{"permanent":false}}
		}`),
	})
	// permanent:true on database should produce one warning
	if countBySeverity(diags, lint.Error) != 0 {
		t.Errorf("expected no errors, got: %v", diags)
	}
	if countBySeverity(diags, lint.Warning) != 1 {
		t.Errorf("expected 1 warning for permanent:true, got %d", countBySeverity(diags, lint.Warning))
	}
}

// --- Lint: schema errors ---

func TestLintInvalidJSON(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/bad.morsel.json", `{not valid json`),
	})
	if countBySeverity(diags, lint.Error) == 0 {
		t.Error("expected error for invalid JSON")
	}
}

func TestLintMissingType(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/api.morsel.json", `{"dockerfile":"Dockerfile"}`),
	})
	if countBySeverity(diags, lint.Error) == 0 {
		t.Error("expected error for missing type")
	}
}

func TestLintMissingDockerfile(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/api.morsel.json", `{"type":"http"}`),
	})
	if countBySeverity(diags, lint.Error) == 0 {
		t.Error("expected error for missing dockerfile")
	}
}

func TestLintInvalidTypeValue(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/api.morsel.json", `{"type":"daemon","dockerfile":"Dockerfile"}`),
	})
	if countBySeverity(diags, lint.Error) == 0 {
		t.Error("expected error for invalid type value")
	}
}

func TestLintHTTPMissingPort(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/api.morsel.json", `{"type":"http","dockerfile":"Dockerfile"}`),
	})
	if countBySeverity(diags, lint.Error) == 0 {
		t.Error("expected error for http missing port")
	}
}

func TestLintCronjobMissingSchedule(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/cron.morsel.json", `{"type":"cronjob","dockerfile":"Dockerfile","timeout":"30m"}`),
	})
	if countBySeverity(diags, lint.Error) == 0 {
		t.Error("expected error for cronjob missing schedule")
	}
}

func TestLintCronjobMissingTimeout(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/cron.morsel.json", `{"type":"cronjob","dockerfile":"Dockerfile","schedule":"0 2 * * *"}`),
	})
	if countBySeverity(diags, lint.Error) == 0 {
		t.Error("expected error for cronjob missing timeout")
	}
}

func TestLintInvalidName(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/api.morsel.json", `{"name":"UPPER","type":"http","dockerfile":"Dockerfile"}`),
	})
	if countBySeverity(diags, lint.Error) == 0 {
		t.Error("expected error for name with uppercase letters")
	}
}

// --- Lint: type-incompatible field warnings ---

func TestLintWorkerPrivateWarning(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/w.morsel.json", `{"type":"worker","dockerfile":"Dockerfile","private":true}`),
	})
	if !hasMessage(diags, "private") {
		t.Error("expected warning about private on worker")
	}
	if countBySeverity(diags, lint.Warning) == 0 {
		t.Error("expected at least one warning")
	}
}

func TestLintWorkerHealthCheckPathWarning(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/w.morsel.json", `{"type":"worker","dockerfile":"Dockerfile","health_check":{"path":"/health"}}`),
	})
	if !hasMessage(diags, "health_check.path") {
		t.Error("expected warning about health_check.path on worker")
	}
}

func TestLintWorkerIdleAfterWarning(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/w.morsel.json", `{"type":"worker","dockerfile":"Dockerfile","idle_after":"1h"}`),
	})
	if !hasMessage(diags, "idle_after") {
		t.Error("expected warning about idle_after on worker")
	}
}

func TestLintCronjobPrivateWarning(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/c.morsel.json", `{"type":"cronjob","dockerfile":"Dockerfile","schedule":"0 2 * * *","timeout":"30m","private":true}`),
	})
	if !hasMessage(diags, "private") {
		t.Error("expected warning about private on cronjob")
	}
}

func TestLintCronjobIdleAfterWarning(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/c.morsel.json", `{"type":"cronjob","dockerfile":"Dockerfile","schedule":"0 2 * * *","timeout":"30m","idle_after":"1h"}`),
	})
	if !hasMessage(diags, "idle_after") {
		t.Error("expected warning about idle_after on cronjob")
	}
}

func TestLintHTTPScheduleWarning(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/api.morsel.json", `{"type":"http","dockerfile":"Dockerfile","schedule":"0 2 * * *"}`),
	})
	if !hasMessage(diags, "schedule") {
		t.Error("expected warning about schedule on http")
	}
}

func TestLintHTTPRetriesWarning(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/api.morsel.json", `{"type":"http","dockerfile":"Dockerfile","retries":2}`),
	})
	if !hasMessage(diags, "retries") {
		t.Error("expected warning about retries on http")
	}
}

// --- Lint: permanent:true warning ---

func TestLintPermanentDatabaseWarning(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/api.morsel.json", `{"type":"http","dockerfile":"Dockerfile","port":8080,"persistence":{"database":{"permanent":true}}}`),
	})
	if !hasMessage(diags, "persistence.database.permanent") {
		t.Error("expected warning about persistence.database.permanent:true")
	}
	if countBySeverity(diags, lint.Error) != 0 {
		t.Error("expected no errors, only warnings")
	}
}

func TestLintPermanentFalseNoWarning(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/api.morsel.json", `{"type":"http","dockerfile":"Dockerfile","port":8080,"persistence":{"database":{"permanent":false}}}`),
	})
	if hasMessage(diags, "permanent") {
		t.Error("expected no warning for permanent:false")
	}
}

// --- Lint: unique name check ---

func TestLintDuplicateNameAcrossFiles(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/a.morsel.json", `{"name":"api","type":"http","dockerfile":"Dockerfile","port":8080}`),
		file(".morsel/b.morsel.json", `{"name":"api","type":"http","dockerfile":"Dockerfile","port":8080}`),
	})
	if countBySeverity(diags, lint.Error) == 0 {
		t.Error("expected error for duplicate name")
	}
	if !hasMessage(diags, `"api"`) {
		t.Error("expected error message to mention the duplicate name")
	}
}

func TestLintUniqueNamesNoError(t *testing.T) {
	l := mustNew(t)
	diags := l.Lint([]lint.File{
		file(".morsel/a.morsel.json", `{"name":"api","type":"http","dockerfile":"Dockerfile","port":8080}`),
		file(".morsel/b.morsel.json", `{"name":"worker","type":"worker","dockerfile":"Dockerfile"}`),
	})
	if countBySeverity(diags, lint.Error) != 0 {
		t.Errorf("expected no errors for unique names, got: %v", diags)
	}
}

func TestLintUnnamedFilesNoUniquenessError(t *testing.T) {
	l := mustNew(t)
	// Files without a name field are not checked for uniqueness.
	diags := l.Lint([]lint.File{
		file(".morsel/a.morsel.json", `{"type":"http","dockerfile":"Dockerfile","port":8080}`),
		file(".morsel/b.morsel.json", `{"type":"worker","dockerfile":"Dockerfile"}`),
	})
	if countBySeverity(diags, lint.Error) != 0 {
		t.Errorf("expected no errors for unnamed files, got: %v", diags)
	}
}

// --- Fix ---

func TestFixReformatsWhitespace(t *testing.T) {
	l := mustNew(t)
	input := `{"type":"http","dockerfile":"Dockerfile"}`
	fixed, err := l.Fix([]lint.File{file(".morsel/api.morsel.json", input)})
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if len(fixed) != 1 {
		t.Fatalf("expected 1 fixed file, got %d", len(fixed))
	}
	got := string(fixed[0].Content)
	// Should have indentation
	if !contains(got, "  ") {
		t.Error("expected indented output")
	}
	// Should end with newline
	if got[len(got)-1] != '\n' {
		t.Error("expected trailing newline")
	}
}

func TestFixCanonicalFieldOrder(t *testing.T) {
	l := mustNew(t)
	// Fields deliberately out of canonical order.
	input := `{
  "dockerfile": "Dockerfile",
  "type": "http",
  "name": "api"
}`
	fixed, err := l.Fix([]lint.File{file(".morsel/api.morsel.json", input)})
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if len(fixed) != 1 {
		t.Fatalf("expected 1 fixed file, got %d", len(fixed))
	}
	got := string(fixed[0].Content)
	// $schema → name → type → dockerfile
	schemaPos := indexOf(got, `"$schema"`)
	namePos := indexOf(got, `"name"`)
	typePos := indexOf(got, `"type"`)
	dockerfilePos := indexOf(got, `"dockerfile"`)
	if schemaPos < 0 || namePos < 0 || typePos < 0 || dockerfilePos < 0 {
		t.Fatal("missing expected fields in fixed output")
	}
	if schemaPos >= namePos || namePos >= typePos || typePos >= dockerfilePos {
		t.Errorf("canonical order violated: $schema=%d name=%d type=%d dockerfile=%d", schemaPos, namePos, typePos, dockerfilePos)
	}
}

func TestFixAlreadyCanonicalNoChange(t *testing.T) {
	l := mustNew(t)
	// Pre-marshal a valid struct to get canonical form.
	input := "{\n  \"$schema\": \"https://raw.githubusercontent.com/ifeanyiecheruo/morsel/v0.1.0/schemas/morsel.schema.json\",\n  \"type\": \"http\",\n  \"dockerfile\": \"Dockerfile\"\n}\n"
	fixed, err := l.Fix([]lint.File{file(".morsel/api.morsel.json", input)})
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if len(fixed) != 0 {
		t.Errorf("expected no fixed files for already-canonical input, got %d", len(fixed))
	}
}

func TestFixAddsSchema(t *testing.T) {
	l := mustNew(t)
	input := "{\n  \"type\": \"http\",\n  \"dockerfile\": \"Dockerfile\"\n}\n"
	fixed, err := l.Fix([]lint.File{file(".morsel/api.morsel.json", input)})
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if len(fixed) != 1 {
		t.Fatalf("expected 1 fixed file, got %d", len(fixed))
	}
	if !contains(string(fixed[0].Content), `"$schema"`) {
		t.Error("expected fixed output to include $schema")
	}
}

func TestFixSkipsFileWithUnknownFields(t *testing.T) {
	l := mustNew(t)
	input := `{"type":"http","dockerfile":"Dockerfile","unknown_field":"value"}`
	fixed, err := l.Fix([]lint.File{file(".morsel/api.morsel.json", input)})
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if len(fixed) != 0 {
		t.Error("expected unknown-field file to be skipped by Fix")
	}
}

func TestFixSkipsInvalidJSON(t *testing.T) {
	l := mustNew(t)
	input := `{not valid`
	fixed, err := l.Fix([]lint.File{file(".morsel/api.morsel.json", input)})
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if len(fixed) != 0 {
		t.Error("expected invalid-JSON file to be skipped by Fix")
	}
}

// indexOf returns the byte index of substr in s, or -1.
func indexOf(s, substr string) int {
	for i := range len(s) - len(substr) + 1 {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
