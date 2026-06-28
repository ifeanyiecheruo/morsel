package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/lint"
	"github.com/spf13/cobra"
)

func (c *cli) lintCmd() *cobra.Command {
	var staged bool
	var fix bool

	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Validate *.morsel.json files in .morsel/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.handler.Lint(cmd.Context(), staged, fix)
		},
	}
	cmd.Flags().BoolVar(&staged, "staged", false, "validate only git-staged files")
	cmd.Flags().BoolVar(&fix, "fix", false, "auto-remediate safe issues")
	return cmd
}

func (h *cliHandler) Lint(_ context.Context, staged, fix bool) error {
	files, err := findMorselFiles(staged)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	linter, err := lint.New()
	if err != nil {
		return fmt.Errorf("initialise linter: %w", err)
	}

	if fix {
		fixed, fixErr := linter.Fix(files)
		if fixErr != nil {
			return fmt.Errorf("fix: %w", fixErr)
		}
		for _, ff := range fixed {
			if writeErr := os.WriteFile(ff.Path, ff.Content, 0o644); writeErr != nil {
				return fmt.Errorf("write %s: %w", ff.Path, writeErr)
			}
			fmt.Printf("fixed: %s\n", ff.Path)
			for idx, f := range files {
				if f.Path == ff.Path {
					files[idx].Content = ff.Content
					break
				}
			}
		}
	}

	diags := linter.Lint(files)

	hasError := false
	for _, diag := range diags {
		sev := "warning"
		if diag.Severity == lint.Error {
			sev = "error"
			hasError = true
		}
		fmt.Printf("%s: %s: %s\n", diag.File, sev, diag.Message)
	}

	if hasError {
		return fmt.Errorf("lint failed")
	}
	return nil
}

func findMorselFiles(staged bool) ([]lint.File, error) {
	if staged {
		return findStagedMorselFiles()
	}
	return findAllMorselFiles()
}

func findAllMorselFiles() ([]lint.File, error) {
	entries, err := os.ReadDir(".morsel")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read .morsel: %w", err)
	}

	var files []lint.File
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".morsel.json") {
			continue
		}
		path := ".morsel/" + entry.Name()
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", path, readErr)
		}
		files = append(files, lint.File{Path: path, Content: content})
	}
	return files, nil
}

func findStagedMorselFiles() ([]lint.File, error) {
	out, err := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=ACM").Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --cached: %w", err)
	}

	var files []lint.File
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" || !strings.HasPrefix(line, ".morsel/") || !strings.HasSuffix(line, ".morsel.json") {
			continue
		}
		content, readErr := os.ReadFile(line)
		if os.IsNotExist(readErr) {
			continue // deleted from disk but still tracked as staged
		}
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", line, readErr)
		}
		files = append(files, lint.File{Path: line, Content: content})
	}
	return files, nil
}
