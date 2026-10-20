package jobs

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// standaloneDirectory reports whether Go has no module or workspace in its ancestry.
func standaloneDirectory(root string) bool {
	if moduleInAncestry(root) {
		return false
	}
	return scanModules(context.Background(), root).standalone
}

// moduleInAncestry treats filesystem errors conservatively as module context.
func moduleInAncestry(root string) bool {
	for directory := filepath.Clean(root); ; directory = filepath.Dir(directory) {
		for _, name := range []string{"go.mod", "go.work"} {
			_, err := os.Stat(filepath.Join(directory, name))
			if err == nil || !os.IsNotExist(err) {
				return true
			}
		}
		if parent := filepath.Dir(directory); parent == directory {
			break
		}
	}
	return false
}

// scanModules finds a nested module and snapshots visited directory mtimes until ctx ends.
func scanModules(ctx context.Context, root string) moduleScan {
	result := moduleScan{standalone: true, dirs: make(map[string]time.Time)}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path != root && entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "vendor") {
			return fs.SkipDir
		}
		if entry.Name() == "go.mod" || entry.Name() == "go.work" {
			result.module = path
			result.standalone = false
			return fs.SkipAll
		}
		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			result.dirs[path] = info.ModTime()
		}
		return nil
	})
	if err != nil {
		result.standalone = false
	}
	result.scannedAt = time.Now()
	return result
}

// valid reports whether the cached module search still matches the tree.
func (scan moduleScan) valid() bool {
	if time.Since(scan.scannedAt) >= 2*time.Second {
		return false
	}
	if scan.module != "" {
		_, err := os.Stat(scan.module)
		return err == nil
	}
	if !scan.standalone {
		return false
	}
	for path, modified := range scan.dirs {
		info, err := os.Stat(path)
		if err != nil || !info.ModTime().Equal(modified) {
			return false
		}
	}
	return true
}

// standaloneDirectory caches subtree scans while rechecking module ancestors; a cancelled caller gets false.
func (m *Manager) standaloneDirectory(ctx context.Context, dir string) bool {
	if moduleInAncestry(dir) {
		return false
	}
	for {
		m.modeMu.Lock()
		entry, ok := m.modeCache[dir]
		if !ok {
			entry = &modeEntry{done: make(chan struct{})}
			m.modeCache[dir] = entry
			m.modeMu.Unlock()
			// A cancelled scan is not standalone, so valid rejects it and the next caller rescans.
			entry.scan = m.scanTree(ctx, dir)
			close(entry.done)
			return entry.scan.standalone
		}
		m.modeMu.Unlock()
		select {
		case <-entry.done:
		case <-ctx.Done():
			return false
		}
		if entry.scan.valid() {
			return entry.scan.standalone
		}
		m.modeMu.Lock()
		if m.modeCache[dir] == entry {
			delete(m.modeCache, dir)
		}
		m.modeMu.Unlock()
	}
}

// configuredGoMode leaves explicit module and workspace settings untouched.
func configuredGoMode(environment []string) bool {
	var module, workspace string
	for _, variable := range environment {
		name, value, ok := strings.Cut(variable, "=")
		if !ok {
			continue
		}
		switch name {
		case "GO111MODULE":
			module = value
		case "GOWORK":
			workspace = value
		}
	}
	return module != "" || (workspace != "" && workspace != "auto" && workspace != "off")
}

// GoEnvironment is what a tool that runs Go from dir, such as Delve, adds to its environment to match Go jobs.
func (m *Manager) GoEnvironment(ctx context.Context, dir string) []string {
	if configuredGoMode(os.Environ()) || !m.standaloneDirectory(ctx, dir) {
		return nil
	}
	return []string{"GO111MODULE=off"}
}

// goCommand creates a command with legacy package lookup for standalone directories.
func (m *Manager) goCommand(ctx context.Context, tool string, args []string) *exec.Cmd {
	return m.goCommandIn(ctx, m.root, tool, args)
}

// goCommandIn uses the module mode of the command's working directory.
func (m *Manager) goCommandIn(ctx context.Context, dir, tool string, args []string) *exec.Cmd {
	command := m.command(ctx, dir, tool, args)
	if !configuredGoMode(command.Environ()) && m.standaloneDirectory(ctx, dir) {
		command.Env = append(command.Environ(), "GO111MODULE=off")
	}
	return command
}

// goEntryCommand uses only the entry directory's ancestry for module mode.
func (m *Manager) goEntryCommand(ctx context.Context, dir, tool string, args []string) *exec.Cmd {
	command := m.command(ctx, dir, tool, args)
	if !configuredGoMode(command.Environ()) && !moduleInAncestry(dir) {
		command.Env = append(command.Environ(), "GO111MODULE=off")
	}
	return command
}

// requestCommand builds the Go process in the request's directory.
func (m *Manager) requestCommand(ctx context.Context, tool string, request Request) *exec.Cmd {
	dir := request.directory(m.root)
	if request.Kind == Run && request.Dir != "" {
		return m.goEntryCommand(ctx, dir, tool, request.Args())
	}
	return m.goCommandIn(ctx, dir, tool, request.Args())
}
