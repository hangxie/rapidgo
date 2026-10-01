package gopls

import (
	"context"
	"os/exec"
	"time"

	"github.com/hangxie/rapidgo/internal/i18n"
)

// Start launches the user's gopls executable and initializes a session.
func Start(ctx context.Context, root string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := exec.LookPath("gopls")
	if err != nil {
		return nil, i18n.Errorf("msg_find_gopls_on_path_w", err)
	}
	processCtx, stop := context.WithCancel(ctx)
	command := exec.CommandContext(processCtx, path, "serve")
	command.Dir = root
	input, err := command.StdoutPipe()
	if err != nil {
		stop()
		return nil, i18n.Errorf("msg_open_gopls_output_w", err)
	}
	output, err := command.StdinPipe()
	if err != nil {
		stop()
		return nil, i18n.Errorf("msg_open_gopls_input_w", err)
	}
	if err := command.Start(); err != nil {
		stop()
		return nil, i18n.Errorf("msg_start_gopls_w", err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	initCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	session, err := Connect(initCtx, input, output, root)
	if err != nil {
		stop()
		<-done
		return nil, err
	}
	session.stopProcess = stop
	session.processDone = done
	return session, nil
}
