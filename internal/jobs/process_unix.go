//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package jobs

import (
	"errors"
	"os/exec"
	"syscall"
)

// configureProcessGroup gives the job its own process group so cancellation
// reaches the program `go run` starts.
func configureProcessGroup(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
}

// interruptProcess asks the job's process group to stop; os/exec kills the
// child if it does not.
func interruptProcess(command *exec.Cmd) error {
	return signalProcessGroup(command, syscall.SIGINT)
}

// killProcessGroup removes any process that outlived the cancelled job.
func killProcessGroup(command *exec.Cmd) {
	_ = signalProcessGroup(command, syscall.SIGKILL)
}

func signalProcessGroup(command *exec.Cmd, signal syscall.Signal) error {
	if command.Process == nil || command.Process.Pid <= 0 {
		return nil
	}
	// Setpgid makes the child's pid its process group id.
	if err := syscall.Kill(-command.Process.Pid, signal); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
