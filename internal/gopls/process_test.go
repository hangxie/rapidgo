package gopls

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStartCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Start(ctx, t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start error = %v, want context cancellation", err)
	}
}

func TestStartReportsMissingGopls(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := Start(context.Background(), t.TempDir())
	require.ErrorContains(t, err, "find gopls on PATH")
}

func TestStartCleansUpProcessThatExitsDuringInitialize(t *testing.T) {
	directory := t.TempDir()
	name := "gopls"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	source := filepath.Join(directory, "stub.go")
	err := os.WriteFile(source, []byte("package main\nfunc main() {}\n"), 0o600)
	require.NoError(t, err)
	command := exec.Command("go", "build", "-o", filepath.Join(directory, name), source)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	t.Setenv("PATH", directory)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = Start(ctx, t.TempDir())
	require.ErrorContains(t, err, "initialize gopls")
}

func TestStartWithInstalledGopls(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls is not installed")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/smoke\n\ngo 1.26.0\n"), 0o600))
	path := filepath.Join(root, "main.go")
	require.NoError(t, os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0o600))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	session, err := Start(ctx, root)
	require.NoError(t, err)
	defer func() { require.NoError(t, session.Close()) }()
	require.NoError(t, session.Open(path, "package main\nfunc main() {}\n"))
	hover, err := session.Hover(ctx, path, Position{Line: 1, Character: 5})
	require.NoError(t, err)
	require.Contains(t, hover, "main")
	require.NoError(t, session.Change(path, "package main\nfunc main() { missing() }\n"))
waitDiagnostics:
	for {
		select {
		case event := <-session.Diagnostics():
			if event.Path != path || event.Version != 2 {
				continue
			}
			found := false
			for _, item := range event.Items {
				if item.Message != "" {
					found = true
				}
			}
			require.True(t, found, "expected a diagnostic for the undefined function")
			break waitDiagnostics
		case <-ctx.Done():
			t.Fatal("gopls did not publish diagnostics for the changed document")
		}
	}
	require.NoError(t, session.CloseDocument(path))
}
