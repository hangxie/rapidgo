//go:build !(aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris)

package jobs

import "os/exec"

// configureProcessGroup has no portable equivalent outside Unix. Cancellation
// stops the go executable itself; a program started by `go run .` may keep
// running until it notices its closed streams.
func configureProcessGroup(*exec.Cmd) {}

func interruptProcess(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return command.Process.Kill()
}

func killProcessGroup(command *exec.Cmd) {
	if command.Process != nil {
		_ = command.Process.Kill()
	}
}
