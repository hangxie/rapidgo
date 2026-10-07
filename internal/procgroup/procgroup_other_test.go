//go:build !(aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris)

package procgroup

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHelperSleep(t *testing.T) {
	if os.Getenv("RAPIDGO_PROCGROUP_SLEEP") == "" {
		t.Skip("helper process only")
	}
	time.Sleep(30 * time.Second)
}

func TestInterruptStopsProcess(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(executable, "-test.run=^TestHelperSleep$")
	command.Env = append(os.Environ(), "RAPIDGO_PROCGROUP_SLEEP=1")
	Configure(command)
	require.NoError(t, command.Start())
	require.NoError(t, Interrupt(command))
	require.Error(t, command.Wait())
	Kill(command)
}

func TestUnstartedCommand(t *testing.T) {
	command := exec.Command("unused")
	require.NoError(t, Interrupt(command))
	Kill(command)
}
