package wellknown

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// BundleSpec loads the OpenAPI document at rootFile from fsys, recursively
// resolves all $ref pointers, and returns the result serialised as JSON.
func BundleSpec(fsys fs.FS, rootFile string) ([]byte, error) {
	resolved, err := resolveFile(fsys, rootFile, map[string]struct{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(resolved)
}

func resolveFile(fsys fs.FS, filePath string, seen map[string]struct{}) (any, error) {
	data, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}
	return resolveRefs(fsys, path.Dir(filePath), v, seen)
}

func resolveRefs(fsys fs.FS, baseDir string, v any, seen map[string]struct{}) (any, error) {
	switch val := v.(type) {
	case map[string]any:
		if ref, ok := val["$ref"].(string); ok {
			return resolveRef(fsys, baseDir, ref, seen)
		}
		out := make(map[string]any, len(val))
		for k, child := range val {
			resolved, err := resolveRefs(fsys, baseDir, child, seen)
			if err != nil {
				return nil, err
			}
			out[k] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(val))
		for i, child := range val {
			resolved, err := resolveRefs(fsys, baseDir, child, seen)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return v, nil
	}
}

func resolveRef(fsys fs.FS, baseDir, ref string, seen map[string]struct{}) (any, error) {
	filePart, fragment, hasFragment := strings.Cut(ref, "#")
	if filePart == "" {
		return nil, fmt.Errorf("same-file $ref not supported: %q", ref)
	}

	// Resolve the file path relative to the referencing file's directory.
	// path.Join cleans ".." so e.g. "paths/../parameters.yaml" → "parameters.yaml".
	absPath := path.Join(baseDir, filePart)

	key := absPath + "#" + fragment
	if _, ok := seen[key]; ok {
		return nil, fmt.Errorf("circular $ref: %q", key)
	}
	seen[key] = struct{}{}
	defer delete(seen, key)

	resolved, err := resolveFile(fsys, absPath, seen)
	if err != nil {
		return nil, err
	}

	if !hasFragment || fragment == "" || fragment == "/" {
		return resolved, nil
	}

	// Navigate the JSON Pointer fragment (RFC 6901), e.g. "/OrgParam".
	v := resolved
	for _, part := range strings.Split(strings.TrimPrefix(fragment, "/"), "/") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("$ref %q: cannot index into %T at %q", ref, v, part)
		}
		child, ok := m[part]
		if !ok {
			return nil, fmt.Errorf("$ref %q: key %q not found", ref, part)
		}
		v = child
	}
	return v, nil
}
