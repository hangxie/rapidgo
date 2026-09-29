package gopls

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// Start launches the user's gopls executable and initializes a session.
func Start(ctx context.Context, root string) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := exec.LookPath("gopls")
	if err != nil {
		return nil, fmt.Errorf("find gopls on PATH: %w", err)
	}
	processCtx, stop := context.WithCancel(ctx)
	command := exec.CommandContext(processCtx, path, "serve")
	command.Dir = root
	input, err := command.StdoutPipe()
	if err != nil {
		stop()
		return nil, fmt.Errorf("open gopls output: %w", err)
	}
	output, err := command.StdinPipe()
	if err != nil {
		stop()
		return nil, fmt.Errorf("open gopls input: %w", err)
	}
	if err := command.Start(); err != nil {
		stop()
		return nil, fmt.Errorf("start gopls: %w", err)
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
