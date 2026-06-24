package local

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

//go:embed migrations/*.secrets.txt
var secretMigrationsFS embed.FS

type secretMigration struct {
	name string
	run  func(ctx context.Context, store *localFileSecretStore) error
}

func runFileMigrations(ctx context.Context, store *localFileSecretStore) error {
	logger := ctxlog.From(ctx)
	fsys, err := fs.Sub(secretMigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("secret migrations fs: %w", err)
	}
	migs, err := loadFileMigrations(fsys)
	if err != nil {
		return fmt.Errorf("load secret migrations: %w", err)
	}
	for _, mig := range migs {
		if err := mig.run(ctx, store); err != nil {
			return fmt.Errorf("secret migration %q: %w", mig.name, err)
		}
		logger.Debug("secret migration ok", "migration", mig.name)
	}
	return nil
}

// fs.ReadDir guarantees lexicographic order, so NNN_ prefixes determine sequence.
func loadFileMigrations(fsys fs.FS) ([]secretMigration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	var all []secretMigration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".secrets.txt") {
			continue
		}
		content, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		migs, err := parseFileMigrationScript(entry.Name(), content)
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
func parseFileMigrationScript(filename string, content []byte) ([]secretMigration, error) {
	var migs []secretMigration
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
			migs = append(migs, secretMigration{
				name: fmt.Sprintf("%s: rename %q → %q", filename, src, dst),
				run:  renameFileSecret(src, dst),
			})
		case "delete":
			if len(fields) != 2 {
				return nil, fmt.Errorf("%s:%d: delete requires one quoted argument", filename, lineNum+1)
			}
			name, err := strconv.Unquote(fields[1])
			if err != nil {
				return nil, fmt.Errorf("%s:%d: invalid secret name: %w", filename, lineNum+1, err)
			}
			migs = append(migs, secretMigration{
				name: fmt.Sprintf("%s: delete %q", filename, name),
				run:  deleteFileSecret(name),
			})
		default:
			return nil, fmt.Errorf("%s:%d: unknown directive %q", filename, lineNum+1, fields[0])
		}
	}
	return migs, nil
}

func renameFileSecret(src, dst string) func(context.Context, *localFileSecretStore) error {
	return func(ctx context.Context, store *localFileSecretStore) error {
		value, err := store.get(ctx, src)
		if errors.Is(err, platform.ErrSecretNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := store.set(ctx, dst, value); err != nil {
			return err
		}
		return store.delete(ctx, src)
	}
}

func deleteFileSecret(name string) func(context.Context, *localFileSecretStore) error {
	return func(ctx context.Context, store *localFileSecretStore) error {
		err := store.delete(ctx, name)
		if errors.Is(err, platform.ErrSecretNotFound) {
			return nil
		}
		return err
	}
}
