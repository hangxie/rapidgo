//go:build !(aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris)

// Package procgroup interrupts and reaps child processes together with their descendants.
package procgroup

import "os/exec"

// Configure has no portable equivalent outside Unix.
func Configure(*exec.Cmd) {}

// Interrupt stops only the command's own process outside Unix.
func Interrupt(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return command.Process.Kill()
}

// Kill stops only the command's own process outside Unix.
func Kill(command *exec.Cmd) {
	if command.Process != nil {
		_ = command.Process.Kill()
	}
}
