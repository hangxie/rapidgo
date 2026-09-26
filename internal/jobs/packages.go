package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Package is one `go list` entry, with only the fields a run target needs.
type Package struct {
	ImportPath string
	Dir        string
	Name       string
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
	command := m.command(ctx, m.root, tool.Path, []string{"list", "-e", "-json", "./..."})
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
