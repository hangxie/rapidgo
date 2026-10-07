//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

// Package procgroup interrupts and reaps child processes together with their descendants.
package procgroup

import (
	"errors"
	"os/exec"
	"syscall"
)

// Configure gives the command its own process group.
func Configure(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
}

// Interrupt asks the command's process group to stop.
func Interrupt(command *exec.Cmd) error {
	return signal(command, syscall.SIGINT)
}

// Kill removes any process in the group that outlived the command.
func Kill(command *exec.Cmd) {
	_ = signal(command, syscall.SIGKILL)
}

func signal(command *exec.Cmd, signal syscall.Signal) error {
	if command.Process == nil || command.Process.Pid <= 0 {
		return nil
	}
	// Setpgid makes the child's pid its process group id.
	if err := syscall.Kill(-command.Process.Pid, signal); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
