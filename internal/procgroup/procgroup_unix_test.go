//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package procgroup

import (
	"bufio"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKillReapsDescendants(t *testing.T) {
	command := exec.Command("sh", "-c", "sleep 30 & echo $!; wait")
	Configure(command)
	output, err := command.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, command.Start())
	line, err := bufio.NewReader(output).ReadString('\n')
	require.NoError(t, err)
	child, err := strconv.Atoi(strings.TrimSpace(line))
	require.NoError(t, err)
	Kill(command)
	require.Error(t, command.Wait())
	require.Eventually(t, func() bool {
		return errors.Is(syscall.Kill(child, 0), syscall.ESRCH)
	}, 3*time.Second, 10*time.Millisecond, "grandchild survived the group kill")
}

func TestInterruptStopsGroup(t *testing.T) {
	command := exec.Command("sleep", "30")
	Configure(command)
	require.NoError(t, command.Start())
	require.NoError(t, Interrupt(command))
	require.Error(t, command.Wait())
	// Signalling a reaped group reports nothing.
	require.NoError(t, Interrupt(command))
}

func TestUnstartedCommand(t *testing.T) {
	command := exec.Command("sleep", "30")
	require.NoError(t, Interrupt(command))
	Kill(command)
}
