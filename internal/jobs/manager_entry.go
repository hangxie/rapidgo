package jobs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/procgroup"
)

// runJobCommand runs one cancellable process and reaps its process group.
func runJobCommand(ctx context.Context, command *exec.Cmd, stdout, stderr io.Writer) error {
	procgroup.Configure(command)
	command.Cancel = func() error { return procgroup.Interrupt(command) }
	command.WaitDelay = killDelay
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if ctx.Err() != nil {
		procgroup.Kill(command)
	}
	return err
}

// buildEntry compiles file-list runs whose first program argument ends in .go.
func (m *Manager) buildEntry(ctx context.Context, tool string, request Request, stdout, stderr io.Writer) (string, func(), error) {
	directory, err := os.MkdirTemp(request.directory(m.root), ".rapidgo-run-")
	if err != nil {
		directory, err = os.MkdirTemp("", "rapidgo-run-")
	}
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	name := "entry"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(directory, name)
	arguments := append([]string{"build", "-o", binary}, request.Files...)
	var command *exec.Cmd
	if request.Dir != "" {
		command = m.goEntryCommand(ctx, request.Dir, tool, arguments)
	} else {
		command = m.goCommandIn(ctx, m.root, tool, arguments)
	}
	if err := runJobCommand(ctx, command, stdout, stderr); err != nil {
		cleanup()
		return "", nil, err
	}
	return binary, cleanup, nil
}

// entryExecutionError explains when the temporary binary cannot execute.
func entryExecutionError(err error, binary string) error {
	if errors.Is(err, fs.ErrPermission) {
		return i18n.Errorf("msg_run_current_entry_from_s_w_directory_may_be_mounted_noexec", filepath.Dir(binary), err)
	}
	return err
}
