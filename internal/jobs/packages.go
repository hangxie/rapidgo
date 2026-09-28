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
	"os"
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
	CgoFiles   []string
	SFiles     []string
	Error      *PackageError
}

// PackageError is a load error reported by go list.
type PackageError struct{ Err string }

// EntryPlan is a file-list run or a package run for sources needing assembly.
type EntryPlan struct {
	Files  []string
	Target string
}

// ErrInactiveEntry means Go excludes the open file under current build constraints.
var ErrInactiveEntry = errors.New("current file is excluded by Go build constraints (GOOS, GOARCH, or tags)")

// ErrAssemblyEntries means file-list runs cannot include assembly with several mains.
var ErrAssemblyEntries = errors.New("assembly sources require a package run, but this package has multiple main files")

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

// DiscoverEntry finds runnable files for one open source file in the background.
func (m *Manager) DiscoverEntry(path string, id uint64) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.wait.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wait.Done()
		plan, err := m.entryPlan(m.ctx, path)
		m.emit(Event{Type: EntryDiscovered, ID: id, EntryPath: path, EntryFiles: plan.Files, EntryTarget: plan.Target, Err: err})
	}()
}

// entryPlan selects active sources or a package target for one main file.
func (m *Manager) entryPlan(ctx context.Context, path string) (EntryPlan, error) {
	if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || sourcePackage(path) != "main" || !hasMainFunction(path) {
		return EntryPlan{}, nil
	}
	tool, err := m.toolchain()
	if err != nil {
		return EntryPlan{}, err
	}
	dir := filepath.Dir(path)
	target := "."
	if relative, relErr := filepath.Rel(m.root, dir); relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		target = "./" + filepath.ToSlash(relative)
	}
	command := m.goCommand(ctx, tool.Path, []string{"list", "-e", "-json", target})
	if target == "." {
		command.Dir = dir
	}
	configureProcessGroup(command)
	command.Cancel = func() error { return interruptProcess(command) }
	command.WaitDelay = killDelay
	var problems bytes.Buffer
	command.Stderr = &problems
	output, err := command.Output()
	if err != nil {
		return EntryPlan{}, listError(err, problems.String())
	}
	var pkg Package
	if err := json.Unmarshal(output, &pkg); err != nil {
		return EntryPlan{}, fmt.Errorf("read go list output: %w", err)
	}
	if pkg.Dir == "" {
		return EntryPlan{}, packageLoadError(pkg)
	}
	if !sameDirectory(pkg.Dir, dir) {
		return EntryPlan{}, fmt.Errorf("go list returned %s for %s", pkg.Dir, dir)
	}
	active := append(append([]string(nil), pkg.GoFiles...), pkg.CgoFiles...)
	if !slices.Contains(active, filepath.Base(path)) {
		if pkg.Error != nil && !strings.HasPrefix(pkg.Error.Err, "build constraints exclude all Go files") && len(active) == 0 {
			return EntryPlan{}, packageLoadError(pkg)
		}
		return EntryPlan{}, ErrInactiveEntry
	}
	entryPackage := sourcePackage(path)
	files := []string{path}
	mains := 0
	for _, name := range active {
		if err := ctx.Err(); err != nil {
			return EntryPlan{}, err
		}
		sibling := filepath.Join(dir, name)
		if sourcePackage(sibling) != entryPackage {
			continue
		}
		if hasMainFunction(sibling) {
			mains++
		} else if name != filepath.Base(path) && !strings.HasSuffix(name, "_test.go") {
			files = append(files, sibling)
		}
	}
	if len(pkg.SFiles) > 0 {
		if mains > 1 {
			return EntryPlan{}, ErrAssemblyEntries
		}
		return EntryPlan{Target: dir}, nil
	}
	return EntryPlan{Files: files}, nil
}

// packageLoadError preserves go list's explanation for an unavailable package.
func packageLoadError(pkg Package) error {
	if pkg.Error != nil && pkg.Error.Err != "" {
		return fmt.Errorf("go list: %s", pkg.Error.Err)
	}
	return errors.New("go list returned no package directory")
}

// sameDirectory accepts Go's resolved path for a symlinked project root.
func sameDirectory(first, second string) bool {
	if filepath.Clean(first) == filepath.Clean(second) {
		return true
	}
	firstInfo, firstErr := os.Stat(first)
	secondInfo, secondErr := os.Stat(second)
	return firstErr == nil && secondErr == nil && os.SameFile(firstInfo, secondInfo)
}

// sourcePackage reads the package clause of a Go source file.
func sourcePackage(path string) string {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly)
	if err != nil || file == nil {
		return ""
	}
	return file.Name.Name
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
	return decodePackages(output)
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
