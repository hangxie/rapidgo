package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/procgroup"
)

// Package is one `go list` entry with sources used to resolve file entries.
type Package struct {
	ImportPath     string
	Dir            string
	Name           string
	GoFiles        []string
	CgoFiles       []string
	IgnoredGoFiles []string
	InvalidGoFiles []string
	SFiles         []string
	CFiles         []string
	CXXFiles       []string
	MFiles         []string
	FFiles         []string
	HFiles         []string
	SwigFiles      []string
	SwigCXXFiles   []string
	SysoFiles      []string
	Error          *PackageError
}

// PackageError is a load error reported by go list.
type PackageError struct{ Err string }

// EntryPlan is a file-list run or a package run for sources needing assembly.
type EntryPlan struct {
	Files  []string
	Target string
}

// ErrInactiveEntry means Go excludes the open file under current build constraints.
var ErrInactiveEntry = i18n.Error("msg_current_file_is_excluded_by_go_build_constraints_goos_goarch_or_tags")

// ErrNativeEntries means file-list runs cannot include native sources with several mains.
var ErrNativeEntries = i18n.Error("msg_native_sources_require_a_package_run_but_this_package_has_multiple_main_files")

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
	if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
		return EntryPlan{}, nil
	}
	entryPackage := sourcePackage(path)
	if entryPackage != "main" || !hasMainFunction(path) {
		return EntryPlan{}, nil
	}
	tool, err := m.toolchain()
	if err != nil {
		return EntryPlan{}, err
	}
	dir := filepath.Dir(path)
	output, err := m.goList(ctx, dir, tool.Path, ".", true)
	if err != nil {
		return EntryPlan{}, err
	}
	var pkg Package
	if err := json.Unmarshal(output, &pkg); err != nil {
		return EntryPlan{}, i18n.Errorf("msg_read_go_list_output_w", err)
	}
	if pkg.Dir == "" {
		return EntryPlan{}, packageLoadError(pkg)
	}
	if !sameDirectory(pkg.Dir, dir) {
		return EntryPlan{}, i18n.Errorf("format_go_list_returned_s_for_s", pkg.Dir, dir)
	}
	active := append(append([]string(nil), pkg.GoFiles...), pkg.CgoFiles...)
	if !slices.Contains(active, filepath.Base(path)) {
		if slices.Contains(pkg.InvalidGoFiles, filepath.Base(path)) {
			return EntryPlan{}, packageLoadError(pkg)
		}
		if slices.Contains(pkg.IgnoredGoFiles, filepath.Base(path)) {
			return EntryPlan{}, ErrInactiveEntry
		}
		if pkg.Error != nil {
			return EntryPlan{}, packageLoadError(pkg)
		}
		return EntryPlan{}, ErrInactiveEntry
	}
	files := []string{path}
	mains := 0
	for _, name := range active {
		if err := ctx.Err(); err != nil {
			return EntryPlan{}, err
		}
		if name == filepath.Base(path) {
			mains++
			continue
		}
		sibling := filepath.Join(dir, name)
		if sourcePackage(sibling) != entryPackage {
			continue
		}
		if hasMainFunction(sibling) {
			mains++
		} else if !strings.HasSuffix(name, "_test.go") {
			files = append(files, sibling)
		}
	}
	if pkg.hasNativeSources() {
		if mains > 1 {
			return EntryPlan{}, ErrNativeEntries
		}
		return EntryPlan{Target: dir}, nil
	}
	return EntryPlan{Files: files}, nil
}

// hasNativeSources reports whether go run needs the whole package for companions.
func (pkg Package) hasNativeSources() bool {
	if len(pkg.SFiles) > 0 || len(pkg.SwigFiles) > 0 || len(pkg.SwigCXXFiles) > 0 {
		return true
	}
	return len(pkg.CgoFiles) > 0 && (len(pkg.CFiles) > 0 || len(pkg.CXXFiles) > 0 ||
		len(pkg.MFiles) > 0 || len(pkg.FFiles) > 0 || len(pkg.HFiles) > 0 || len(pkg.SysoFiles) > 0)
}

// packageLoadError preserves go list's explanation for an unavailable package.
func packageLoadError(pkg Package) error {
	if pkg.Error != nil && pkg.Error.Err != "" {
		return i18n.Errorf("format_go_list_s", pkg.Error.Err)
	}
	return i18n.Error("msg_go_list_returned_no_package_directory")
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
	output, err := m.goList(ctx, m.root, tool.Path, "./...", false)
	if err != nil {
		return nil, err
	}
	return decodePackages(output)
}

// goList runs a cancellable package listing in dir.
func (m *Manager) goList(ctx context.Context, dir, tool, target string, entry bool) ([]byte, error) {
	args := []string{"list", "-e", "-json", target}
	var command *exec.Cmd
	if entry {
		command = m.goEntryCommand(ctx, dir, tool, args)
	} else {
		command = m.goCommandIn(ctx, dir, tool, args)
	}
	procgroup.Configure(command)
	command.Cancel = func() error { return procgroup.Interrupt(command) }
	command.WaitDelay = killDelay
	var problems bytes.Buffer
	command.Stderr = &problems
	output, err := command.Output()
	if err != nil {
		return nil, listError(err, problems.String())
	}
	return output, nil
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
		return i18n.Errorf("format_go_list_s_w", problems, err)
	}
	return i18n.Errorf("format_go_list_w", err)
}

// decodePackages reads the packages from `go list -json`, sorted.
func decodePackages(output []byte) ([]Package, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	var packages []Package
	var loadError error
	for {
		var listed Package
		if err := decoder.Decode(&listed); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, i18n.Errorf("msg_read_go_list_output_w", err)
		}
		if listed.Dir != "" {
			packages = append(packages, listed)
		} else if listed.Error != nil && listed.Error.Err != "" {
			loadError = errors.Join(loadError, packageLoadError(listed))
		}
	}
	if len(packages) == 0 && loadError != nil {
		return nil, loadError
	}
	sort.Slice(packages, func(first, second int) bool {
		return packages[first].ImportPath < packages[second].ImportPath
	})
	return packages, nil
}
