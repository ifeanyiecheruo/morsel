package secrets

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/ifeanyiecheruo/morsel/platform"
)

type migration struct {
	name string
	run  func(ctx context.Context, store platform.SecretStore) error
}

// fs.ReadDir guarantees lexicographic order, so NNN_ prefixes determine sequence.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	var all []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".secrets.txt") {
			continue
		}
		content, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		migs, err := parseMigrationScript(entry.Name(), content)
		if err != nil {
			return nil, err
		}
		all = append(all, migs...)
	}
	return all, nil
}

// Supported directives:
//
//	rename "old-name" "new-name"
//	delete "name"
func parseMigrationScript(filename string, content []byte) ([]migration, error) {
	var migs []migration
	for lineNum, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		switch fields[0] {
		case "rename":
			if len(fields) != 3 {
				return nil, fmt.Errorf("%s:%d: rename requires two quoted arguments", filename, lineNum+1)
			}
			src, err := strconv.Unquote(fields[1])
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid source name: %w", filename, lineNum+1, err)
			}
			dst, err := strconv.Unquote(fields[2])
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid dest name: %w", filename, lineNum+1, err)
			}
			migs = append(migs, migration{
				name: fmt.Sprintf("%s: rename %q → %q", filename, src, dst),
				run:  renameSecret(src, dst),
			})
		case "delete":
			if len(fields) != 2 {
				return nil, fmt.Errorf("%s:%d: delete requires one quoted argument", filename, lineNum+1)
			}
			name, err := strconv.Unquote(fields[1])
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid secret name: %w", filename, lineNum+1, err)
			}
			migs = append(migs, migration{
				name: fmt.Sprintf("%s: delete %q", filename, name),
				run:  deleteSecret(name),
			})
		default:
			return nil, fmt.Errorf("%s:%d: unknown directive %q", filename, lineNum+1, fields[0])
		}
	}
	return migs, nil
}

// If src is absent this is a no-op.
func renameSecret(src, dst string) func(context.Context, platform.SecretStore) error {
	return func(ctx context.Context, store platform.SecretStore) error {
		value, err := store.Get(ctx, src)
		if errors.Is(err, platform.ErrSecretNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := store.Set(ctx, dst, value); err != nil {
			return err
		}
		return store.Delete(ctx, src)
	}
}

// If name is absent this is a no-op.
func deleteSecret(name string) func(context.Context, platform.SecretStore) error {
	return func(ctx context.Context, store platform.SecretStore) error {
		err := store.Delete(ctx, name)
		if errors.Is(err, platform.ErrSecretNotFound) {
			return nil
		}
		return err
	}
}
