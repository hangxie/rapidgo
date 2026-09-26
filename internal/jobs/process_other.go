//go:build !(aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris)

package jobs

import "os/exec"

// configureProcessGroup has no portable equivalent outside Unix, so cancelling
// stops the go executable but may leave a `go run` program behind.
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
