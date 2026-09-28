package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Package is one `go list` entry with sources used to resolve file entries.
type Package struct {
	ImportPath string
	Dir        string
	Name       string
	GoFiles    []string
	MainFiles  []string
}

// FilesFor returns an entry file and the sources it can share with other mains.
func (listed Package) FilesFor(entry string) []string {
	if !slices.Contains(listed.MainFiles, entry) {
		return nil
	}
	files := []string{entry}
	for _, name := range listed.GoFiles {
		if !strings.HasSuffix(name, "_test.go") && !slices.Contains(listed.MainFiles, name) {
			files = append(files, name)
		}
	}
	return files
}

// Discover lists the module's packages in the background, as a Discovered event.
func (m *Manager) Discover() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.wait.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wait.Done()
		packages, err := m.packages(m.ctx)
		m.emit(Event{Type: Discovered, Packages: packages, Err: err})
	}()
}

// packages asks `go list` what the module contains, errors included.
func (m *Manager) packages(ctx context.Context) ([]Package, error) {
	tool, err := m.toolchain()
	if err != nil {
		return nil, err
	}
	command := m.goCommand(ctx, tool.Path, []string{"list", "-e", "-json", "./..."})
	configureProcessGroup(command)
	command.Cancel = func() error { return interruptProcess(command) }
	command.WaitDelay = killDelay
	var problems bytes.Buffer
	command.Stderr = &problems
	output, err := command.Output()
	if err != nil {
		return nil, listError(err, problems.String())
	}
	packages, err := decodePackages(output)
	if err != nil {
		return packages, err
	}
	for index := range packages {
		if packages[index].Name != "main" {
			continue
		}
		for _, name := range packages[index].GoFiles {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if hasMainFunction(filepath.Join(packages[index].Dir, name)) {
				packages[index].MainFiles = append(packages[index].MainFiles, name)
			}
		}
	}
	return packages, nil
}

// hasMainFunction checks a source file for a top-level main function.
func hasMainFunction(path string) bool {
	// A partial tree can identify main; the Go job reports syntax errors.
	file, _ := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if file == nil {
		return false
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Recv == nil && function.Name.Name == "main" {
			return true
		}
	}
	return false
}

func listError(err error, problems string) error {
	problems = strings.TrimSpace(problems)
	if index := strings.IndexAny(problems, "\r\n"); index >= 0 {
		problems = strings.TrimSpace(problems[:index])
	}
	if problems != "" {
		return fmt.Errorf("go list: %s: %w", problems, err)
	}
	return fmt.Errorf("go list: %w", err)
}

// decodePackages reads the packages from `go list -json`, sorted.
func decodePackages(output []byte) ([]Package, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	var packages []Package
	for {
		var listed Package
		if err := decoder.Decode(&listed); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("read go list output: %w", err)
		}
		if listed.Dir != "" {
			packages = append(packages, listed)
		}
	}
	sort.Slice(packages, func(first, second int) bool {
		return packages[first].ImportPath < packages[second].ImportPath
	})
	return packages, nil
}
