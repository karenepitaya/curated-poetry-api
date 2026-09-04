package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	poetry "github.com/karenepitaya/curated-poetry-api"
)

func main() {
	var fileFlags fileValues
	var allowMissing bool
	flag.Var(&fileFlags, "files", "validate a work JSON file; may be repeated, and positional paths must come last")
	flag.BoolVar(&allowMissing, "allow-missing", false, "allow missing changed paths and run full-corpus validation; must precede positional paths")
	flag.Parse()

	files := append([]string(nil), fileFlags...)
	files = append(files, flag.Args()...)
	if len(fileFlags) == 0 && len(files) > 0 {
		fail("positional paths require --files before the first path")
	}
	if allowMissing && len(fileFlags) == 0 {
		fail("--allow-missing requires --files")
	}

	normalized := make([]string, 0, len(files))
	for _, name := range files {
		pathName, err := corpusPath(name)
		if err != nil {
			fail(err.Error())
		}
		normalized = append(normalized, pathName)
	}
	checkFiles, full, err := planCheck(os.DirFS("."), normalized, allowMissing)
	if err != nil {
		fail(err.Error())
	}
	if err := poetry.CheckFilesFS(os.DirFS("."), checkFiles); err != nil {
		fail(err.Error())
	}
	if full {
		fmt.Println("corpuscheck: full corpus valid")
		return
	}
	fmt.Printf("corpuscheck: %d work file(s) valid\n", len(checkFiles))
}

type fileValues []string

func (values *fileValues) String() string {
	return strings.Join(*values, ",")
}

func (values *fileValues) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func planCheck(dataFS fs.FS, files []string, allowMissing bool) ([]string, bool, error) {
	if len(files) == 0 {
		return nil, true, nil
	}

	workFiles := make([]string, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	full := false
	for _, name := range files {
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		if _, err := fs.Stat(dataFS, name); err != nil {
			if errors.Is(err, fs.ErrNotExist) && allowMissing {
				full = true
				continue
			}
			return nil, false, fmt.Errorf("stat %s: %w", name, err)
		}
		if !strings.HasPrefix(name, "corpus/works/") || path.Ext(name) != ".json" {
			full = true
			continue
		}
		workFiles = append(workFiles, name)
	}
	if full {
		return nil, true, nil
	}
	return workFiles, false, nil
}

func corpusPath(name string) (string, error) {
	cleaned := filepath.Clean(name)
	if filepath.IsAbs(cleaned) {
		root, err := filepath.Abs(".")
		if err != nil {
			return "", fmt.Errorf("resolve repository root: %w", err)
		}
		relative, err := filepath.Rel(root, cleaned)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", name, err)
		}
		if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("path is outside repository: %s", name)
		}
		cleaned = relative
	}
	return filepath.ToSlash(cleaned), nil
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "corpuscheck:", message)
	os.Exit(1)
}
