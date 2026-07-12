// go-deps manages code-generation metadata via .gen.json sidecar files.
//
// Subcommands:
//
//	gen <file.gen.json>   – run the declared generator and touch <file>.gen.json.stamp
//	get <file.gen.json>   – print repo-relative input paths for one generator
//	get <dir>...          – print embed files + stamp files needed to build the dirs
//	lint <dir>...         – verify every //go:generate uses go-deps gen and its json exists
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type genDef struct {
	Cmd     string   `json:"cmd"`
	Inputs  []string `json:"inputs"`
	Outputs []string `json:"outputs"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go-deps <gen|get|lint> ...")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "gen":
		runGen(os.Args[2:])
	case "get":
		runGet(os.Args[2:])
	case "lint":
		runLint(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "go-deps: unknown subcommand %q\n", os.Args[1])
		os.Exit(1)
	}
}

// runGen runs the generator declared in the .gen.json file and touches its stamp.
func runGen(args []string) {
	if len(args) == 0 {
		die("gen: missing .gen.json argument")
	}
	jsonFile := args[0]
	g := mustParseGen(jsonFile)

	jsonDir := filepath.Dir(jsonFile)
	if jsonDir == "" {
		jsonDir = "."
	}

	parts := strings.Fields(g.Cmd)
	if len(parts) == 0 {
		die("gen: empty cmd in %s", jsonFile)
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = jsonDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		die("gen: %v", err)
	}

	// Touch the stamp file.
	stamp := jsonFile + ".stamp"
	f, err := os.Create(stamp)
	if err != nil {
		die("gen: create stamp %s: %v", stamp, err)
	}
	if err := f.Close(); err != nil {
		die("gen: close stamp %s: %v", stamp, err)
	}
}

// runGet prints space-separated repo-relative dependency paths.
// With a single .gen.json argument it prints the generator's input files.
// With directory arguments it prints embed files + .gen.json.stamp files.
func runGet(args []string) {
	if len(args) == 0 {
		die("get: missing arguments")
	}

	if strings.HasSuffix(args[0], ".gen.json") {
		// Single-generator mode: expand and print input files.
		// The json file itself is always included — editing it invalidates the stamp.
		jsonFile := args[0]
		g := mustParseGen(jsonFile)
		jsonDir := filepath.Dir(jsonFile)
		if jsonDir == "" {
			jsonDir = "."
		}
		seen := map[string]bool{filepath.ToSlash(jsonFile): true}
		for _, pattern := range g.Inputs {
			expandPath(jsonDir, pattern, seen)
		}
		printSorted(seen)
		return
	}

	// Directory mode: collect .go files, embed files, and generator stamp files.
	srcDirs := args
	goFiles := map[string]bool{}
	embedFiles := map[string]bool{}
	jsonRefs := map[string]bool{} // absolute/CWD-relative paths to referenced .gen.json files

	for _, dir := range srcDirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			goFiles[filepath.ToSlash(path)] = true
			scanGoFile(path, embedFiles, jsonRefs)
			return nil
		})
	}

	// Add a stamp for each generator whose outputs fall under the source dirs.
	stamps := map[string]bool{}
	for jsonFile := range jsonRefs {
		g, err := parseGen(jsonFile)
		if err != nil {
			continue
		}
		jsonDir := filepath.Dir(jsonFile)
		if jsonDir == "" {
			jsonDir = "."
		}
		if outputsOverlapDirs(g.Outputs, jsonDir, srcDirs) {
			stamps[filepath.ToSlash(jsonFile)+".stamp"] = true
		}
	}

	all := map[string]bool{}
	for f := range goFiles {
		all[f] = true
	}
	for f := range embedFiles {
		all[f] = true
	}
	for s := range stamps {
		all[s] = true
	}
	printSorted(all)
}

// runLint checks that every //go:generate directive uses go-deps gen and that
// the referenced .gen.json file exists.
func runLint(args []string) {
	var goFiles []string
	for _, arg := range args {
		// Try glob first; fall back to directory walk.
		if matches, _ := filepath.Glob(arg); len(matches) > 0 {
			for _, m := range matches {
				if strings.HasSuffix(m, ".go") {
					goFiles = append(goFiles, m)
				}
			}
		} else {
			_ = filepath.WalkDir(arg, func(path string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() && strings.HasSuffix(path, ".go") {
					goFiles = append(goFiles, path)
				}
				return nil
			})
		}
	}

	ok := true
	for _, goFile := range goFiles {
		src, err := os.ReadFile(goFile)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "//go:generate ") {
				continue
			}
			rest := strings.TrimPrefix(trimmed, "//go:generate ")
			if !strings.HasPrefix(rest, "go-deps gen ") {
				fmt.Fprintf(os.Stderr, "%s:%d: //go:generate must use go-deps gen, got: %s\n",
					goFile, i+1, rest)
				ok = false
				continue
			}
			ref := strings.Fields(strings.TrimPrefix(rest, "go-deps gen "))[0]
			jsonFile := filepath.Join(filepath.Dir(goFile), ref)
			if _, statErr := os.Stat(jsonFile); os.IsNotExist(statErr) {
				fmt.Fprintf(os.Stderr, "%s:%d: referenced %s does not exist\n",
					goFile, i+1, jsonFile)
				ok = false
			}
		}
	}
	if !ok {
		os.Exit(1)
	}
}

// scanGoFile collects //go:embed paths and //go:generate go-deps gen references.
func scanGoFile(path string, embedFiles, jsonRefs map[string]bool) {
	src, err := os.ReadFile(path)
	if err != nil {
		return
	}
	fileDir := filepath.Dir(path)
	for _, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "//go:embed "):
			raw := strings.TrimPrefix(trimmed, "//go:embed ")
			raw = strings.ReplaceAll(raw, "all:", "")
			for _, pattern := range strings.Fields(raw) {
				expandPath(fileDir, pattern, embedFiles)
			}
		case strings.HasPrefix(trimmed, "//go:generate go-deps gen "):
			ref := strings.Fields(strings.TrimPrefix(trimmed, "//go:generate go-deps gen "))[0]
			jsonPath := filepath.ToSlash(filepath.Join(fileDir, ref))
			jsonRefs[jsonPath] = true
		}
	}
}

// expandPath expands a single glob pattern (or plain path/directory) relative
// to baseDir and records all matching files in seen using forward-slash paths.
func expandPath(baseDir, pattern string, seen map[string]bool) {
	full := filepath.Join(baseDir, pattern)
	matches, err := filepath.Glob(full)
	if err != nil || len(matches) == 0 {
		// No glob matches — treat as a literal path/directory.
		info, statErr := os.Stat(full)
		if statErr != nil {
			return
		}
		if info.IsDir() {
			walkDir(full, seen)
		} else {
			seen[filepath.ToSlash(full)] = true
		}
		return
	}
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if info.IsDir() {
			walkDir(m, seen)
		} else {
			seen[filepath.ToSlash(m)] = true
		}
	}
}

func walkDir(dir string, seen map[string]bool) {
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			seen[filepath.ToSlash(p)] = true
		}
		return nil
	})
}

// outputsOverlapDirs reports whether any output from the generator falls under
// at least one of the given source directories.
func outputsOverlapDirs(outputs []string, jsonDir string, srcDirs []string) bool {
	for _, out := range outputs {
		full := filepath.Join(jsonDir, out)
		matches, _ := filepath.Glob(full)
		if len(matches) > 0 {
			for _, m := range matches {
				for _, d := range srcDirs {
					if isUnder(filepath.ToSlash(m), filepath.ToSlash(d)) {
						return true
					}
				}
			}
			continue
		}
		// No existing matches — check the static prefix of the pattern.
		prefix := strings.SplitN(out, "*", 2)[0]
		prefix = strings.TrimSuffix(prefix, "/")
		candidate := filepath.ToSlash(filepath.Clean(filepath.Join(jsonDir, prefix)))
		for _, d := range srcDirs {
			if isUnder(candidate, filepath.ToSlash(d)) {
				return true
			}
		}
	}
	return false
}

func isUnder(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+"/")
}

func mustParseGen(path string) *genDef {
	g, err := parseGen(path)
	if err != nil {
		die("%v", err)
	}
	return g
}

func parseGen(path string) (*genDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("parseGen: %w", err)
	}
	var g genDef
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parseGen %s: %w", path, err)
	}
	return &g, nil
}

func printSorted(m map[string]bool) {
	paths := make([]string, 0, len(m))
	for p := range m {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	fmt.Print(strings.Join(paths, " "))
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "go-deps: "+format+"\n", args...)
	os.Exit(1)
}
