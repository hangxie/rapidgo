package jobs

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverEntryReportsRunPlan(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/entry\n\ngo 1.23\n"), 0o600))
	entry := filepath.Join(root, "main.go")
	helper := filepath.Join(root, "helper.go")
	require.NoError(t, os.WriteFile(entry, []byte("package main\nfunc main() { helper() }\n"), 0o600))
	require.NoError(t, os.WriteFile(helper, []byte("package main\nfunc helper() {}\n"), 0o600))
	manager := NewManager(root)
	t.Cleanup(manager.Close)
	manager.DiscoverEntry(entry, 42)
	select {
	case event := <-manager.Events():
		require.Equal(t, EntryDiscovered, event.Type)
		require.NoError(t, event.Err)
		assert.EqualValues(t, 42, event.ID)
		assert.Equal(t, entry, event.EntryPath)
		assert.Equal(t, []string{entry, helper}, event.EntryFiles)
		assert.Empty(t, event.EntryTarget)
	case <-time.After(10 * time.Second):
		t.Fatal("current-entry discovery did not complete")
	}
}

func TestPackageListHelperProcess(t *testing.T) {
	if payload := os.Getenv("RAPIDGO_PACKAGE_LIST_TEST"); payload != "" {
		_, _ = os.Stdout.WriteString(payload)
		os.Exit(0)
	}
}

func TestEntryPlanRejectsBadPackageListings(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "main.go")
	require.NoError(t, os.WriteFile(entry, []byte("package main\nfunc main() {}\n"), 0o600))
	for _, test := range []struct {
		name, payload, want string
	}{
		{name: "malformed", payload: `{`, want: "read go list output"},
		{name: "load error", payload: packageJSON(t, Package{Error: &PackageError{Err: "bad build tag"}}), want: "bad build tag"},
		{name: "wrong directory", payload: packageJSON(t, Package{Dir: filepath.Join(root, "other"), GoFiles: []string{"main.go"}}), want: "go list returned"},
		{name: "ignored file", payload: packageJSON(t, Package{Dir: root, IgnoredGoFiles: []string{"main.go"}}), want: ErrInactiveEntry.Error()},
		{name: "package failure", payload: packageJSON(t, Package{Dir: root, Error: &PackageError{Err: "imports failed"}}), want: "imports failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := NewManager(root)
			t.Cleanup(manager.Close)
			manager.detected.Do(func() { manager.tool = Toolchain{Path: "go"} })
			manager.command = func(ctx context.Context, dir, _ string, _ []string) *exec.Cmd {
				command := exec.CommandContext(ctx, os.Args[0], "-test.run=TestPackageListHelperProcess")
				command.Dir = dir
				command.Env = append(os.Environ(), "RAPIDGO_PACKAGE_LIST_TEST="+test.payload)
				return command
			}
			_, err := manager.entryPlan(t.Context(), entry)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func packageJSON(t *testing.T, pkg Package) string {
	t.Helper()
	data, err := json.Marshal(pkg)
	require.NoError(t, err)
	return string(data)
}
