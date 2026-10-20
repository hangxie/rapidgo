package dap

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hangxie/rapidgo/internal/procgroup"
)

const stubSource = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	if strings.Join(os.Args[1:], " ") != "dap --listen=127.0.0.1:0" {
		fmt.Fprintln(os.Stderr, "unexpected arguments", os.Args[1:])
		os.Exit(3)
	}
	mode := os.Getenv("RAPIDGO_STUB_MODE")
	switch mode {
	case "exit":
		for line := range 5 {
			fmt.Fprintln(os.Stderr, "noise", line)
		}
		fmt.Fprintln(os.Stderr, "boom: cannot start")
		os.Exit(1)
	case "silent":
		time.Sleep(time.Minute)
		return
	case "longline":
		fmt.Print(strings.Repeat("x", 2<<20))
		time.Sleep(20 * time.Second)
		return
	case "detach":
		// A child in its own process group keeps stdout open after this process exits.
		child := exec.Command(os.Args[0], os.Args[1:]...)
		child.Env = append(os.Environ(), "RAPIDGO_STUB_MODE=silent")
		child.Stdout = os.Stdout
		child.SysProcAttr = &syscall.SysProcAttr{}
		if field := reflect.ValueOf(child.SysProcAttr).Elem().FieldByName("Setpgid"); field.IsValid() {
			field.SetBool(true)
		}
		if err := child.Start(); err != nil {
			panic(err)
		}
		fmt.Fprintln(os.Stderr, "child", child.Process.Pid)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	fmt.Println("preamble")
	fmt.Println("DAP server listening at:", listener.Addr())
	fmt.Fprintln(os.Stderr, "WARNING: stub")
	conn, err := listener.Accept()
	if err != nil {
		panic(err)
	}
	reader := bufio.NewReader(conn)
	seq := 0
	for {
		length := 0
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if line == "\r\n" {
				break
			}
			if value, ok := strings.CutPrefix(line, "Content-Length: "); ok {
				length, _ = strconv.Atoi(strings.TrimSpace(value))
			}
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(reader, body); err != nil {
			return
		}
		var request struct {
			Seq     int    ` + "`json:\"seq\"`" + `
			Command string ` + "`json:\"command\"`" + `
		}
		_ = json.Unmarshal(body, &request)
		if request.Command == "disconnect" && mode == "hang" {
			continue
		}
		seq++
		data, _ := json.Marshal(map[string]any{"seq": seq, "type": "response", "request_seq": request.Seq, "command": request.Command, "success": true})
		fmt.Fprintf(conn, "Content-Length: %d\r\n\r\n%s", len(data), data)
		if pid, _ := strconv.Atoi(os.Getenv("RAPIDGO_STUB_DEBUGGEE")); request.Command == "initialize" && pid > 0 {
			seq++
			data, _ = json.Marshal(map[string]any{"seq": seq, "type": "event", "event": "process", "body": map[string]any{"systemProcessId": pid}})
			fmt.Fprintf(conn, "Content-Length: %d\r\n\r\n%s", len(data), data)
		}
		if request.Command == "disconnect" {
			_ = conn.Close()
			return
		}
		if mode == "deaf" {
			// Stop reading so the client's writes fill the socket.
			time.Sleep(time.Minute)
			return
		}
	}
}
`

// installStub puts a scripted dlv first on PATH for the given mode.
func installStub(t *testing.T, mode string) {
	t.Helper()
	directory := t.TempDir()
	source := filepath.Join(directory, "stub.go")
	require.NoError(t, os.WriteFile(source, []byte(stubSource), 0o600))
	name := "dlv"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	command := exec.Command("go", "build", "-o", filepath.Join(directory, name), source)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	t.Setenv("PATH", directory)
	t.Setenv("RAPIDGO_STUB_MODE", mode)
}

func TestStartCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Start(ctx, t.TempDir())
	require.ErrorIs(t, err, context.Canceled)
}

func TestStartReportsMissingDlv(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := Start(context.Background(), t.TempDir())
	require.ErrorContains(t, err, "find dlv on PATH")
}

func TestStartReportsEarlyExit(t *testing.T) {
	installStub(t, "exit")
	_, err := Start(context.Background(), t.TempDir())
	require.EqualError(t, err, "dlv exited before accepting connections: noise 1\nnoise 2\nnoise 3\nnoise 4\nboom: cannot start")
}

func TestStartGivesUpWhenAdapterNeverListens(t *testing.T) {
	installStub(t, "silent")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	began := time.Now()
	_, err := Start(ctx, t.TempDir())
	require.ErrorContains(t, err, "wait for dlv to listen")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(began), 5*time.Second)
}

func TestStartForwardsAdapterOutputAndDisconnects(t *testing.T) {
	installStub(t, "serve")
	session, err := Start(context.Background(), t.TempDir())
	require.NoError(t, err)
	var console, stderr bool
	timeout := time.After(5 * time.Second)
	for !console || !stderr {
		select {
		case event := <-session.Events():
			console = console || reflect.DeepEqual(event, Event{Name: "output", Category: "console", Output: "preamble\n"})
			stderr = stderr || reflect.DeepEqual(event, Event{Name: "output", Category: "stderr", Output: "WARNING: stub\n"})
		case <-timeout:
			t.Fatal("adapter output was not forwarded")
		}
	}
	require.NoError(t, session.Close())
	for range session.Events() {
	}
	require.NoError(t, session.Close())
}

func TestCloseKillsUnresponsiveAdapter(t *testing.T) {
	installStub(t, "hang")
	session, err := Start(context.Background(), t.TempDir())
	require.NoError(t, err)
	closed := make(chan struct{})
	go func() {
		_ = session.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not kill an adapter that ignores disconnect")
	}
	for range session.Events() {
	}
}

func TestStartFailsWhenAdapterOutputBreaks(t *testing.T) {
	installStub(t, "longline")
	errs := make(chan error, 1)
	go func() {
		_, err := Start(context.Background(), t.TempDir())
		errs <- err
	}()
	select {
	case err := <-errs:
		require.ErrorContains(t, err, "dlv exited before accepting connections")
	case <-time.After(5 * time.Second):
		t.Fatal("Start waited for an adapter whose output stopped being readable")
	}
}

func TestCloseKillsDebuggeeWhenAdapterHangs(t *testing.T) {
	installStub(t, "hang")
	path, err := exec.LookPath("dlv")
	require.NoError(t, err)
	debuggee := exec.Command(path, "dap", "--listen=127.0.0.1:0")
	debuggee.Env = append(os.Environ(), "RAPIDGO_STUB_MODE=silent")
	procgroup.Configure(debuggee)
	require.NoError(t, debuggee.Start())
	exited := make(chan struct{})
	go func() {
		_ = debuggee.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		_ = debuggee.Process.Kill()
		<-exited
	})
	t.Setenv("RAPIDGO_STUB_DEBUGGEE", strconv.Itoa(debuggee.Process.Pid))
	session, err := Start(context.Background(), t.TempDir())
	require.NoError(t, err)
	awaitEvent(context.Background(), t, session.Events(), "process")
	require.NoError(t, session.Close())
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("debuggee outlived an adapter that ignored disconnect")
	}
}

func TestCloseDoesNotWaitForDetachedOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("detaching a child into its own process group needs Unix")
	}
	installStub(t, "detach")
	session, err := Start(context.Background(), t.TempDir())
	require.NoError(t, err)
	child := awaitEvent(context.Background(), t, session.Events(), "output", func(event Event) bool {
		return strings.HasPrefix(event.Output, "child ")
	})
	pid, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(child.Output, "child ")))
	require.NoError(t, err)
	t.Cleanup(func() {
		if process, err := os.FindProcess(pid); err == nil {
			_ = process.Kill()
		}
	})
	closed := make(chan struct{})
	go func() {
		_ = session.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close waited for a detached process holding the adapter output")
	}
}

func TestCloseInterruptsBlockedWrite(t *testing.T) {
	installStub(t, "deaf")
	session, err := Start(context.Background(), t.TempDir())
	require.NoError(t, err)
	go func() {
		for range session.Events() {
		}
	}()
	// The adapter reads nothing after initialize, so this write blocks once the socket buffers fill.
	go func() { _, _ = session.Evaluate(context.Background(), strings.Repeat("x", 8<<20), 0, "repl") }()
	time.Sleep(200 * time.Millisecond)
	closed := make(chan struct{})
	go func() {
		_ = session.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close waited behind a write the adapter never reads")
	}
}

// startInstalled launches the user's dlv on a small program, skipping when dlv cannot debug it.
func startInstalled(t *testing.T) (context.Context, *Session, <-chan Event, string) {
	t.Helper()
	if _, err := exec.LookPath("dlv"); err != nil {
		t.Skip("dlv is not installed")
	}
	// Launch must undo a user's -trimpath, which strips the source paths breakpoints need.
	t.Setenv("GOFLAGS", "-trimpath")
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/smoke\n\ngo 1.26.0\n"), 0o600))
	source := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tx := 41\n\tx++\n\tfmt.Println(\"value\", x)\n}\n"
	path := filepath.Join(root, "main.go")
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	session, err := Start(ctx, root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	events := make(chan Event, 1024)
	go func() {
		for event := range session.Events() {
			events <- event
		}
		close(events)
	}()
	if err := session.Launch(ctx, LaunchConfig{Mode: ModeDebug, Program: root, Cwd: root}); err != nil {
		if strings.Contains(err.Error(), "too old") {
			t.Skipf("installed dlv cannot debug this Go version: %v", err)
		}
		require.NoError(t, err)
	}
	breakpoints, err := session.SetBreakpoints(ctx, path, []SourceBreakpoint{{Line: 8}})
	require.NoError(t, err)
	require.Len(t, breakpoints, 1)
	require.True(t, breakpoints[0].Verified)
	require.NoError(t, session.ConfigurationDone(ctx))
	return ctx, session, events, path
}

func TestStartWithInstalledDlv(t *testing.T) {
	ctx, session, events, _ := startInstalled(t)
	stopped := awaitEvent(ctx, t, events, "stopped")
	require.Equal(t, "breakpoint", stopped.Reason)
	frames, _, err := session.StackTrace(ctx, stopped.ThreadID, 0, 1)
	require.NoError(t, err)
	require.Equal(t, 8, frames[0].Line)
	require.Equal(t, "main.main", frames[0].Name)
	scopes, err := session.Scopes(ctx, frames[0].ID)
	require.NoError(t, err)
	locals, err := session.Variables(ctx, scopes[0].Reference, 0, 0)
	require.NoError(t, err)
	require.Contains(t, locals, Variable{Name: "x", Value: "42", Type: "int", EvaluateName: "x"})
	value, err := session.Evaluate(ctx, "x*2", frames[0].ID, "watch")
	require.NoError(t, err)
	require.Equal(t, "84", value.Value)
	_, err = session.SetVariable(ctx, scopes[0].Reference, "x", "7")
	require.NoError(t, err)
	require.NoError(t, session.Continue(ctx, stopped.ThreadID))
	output := awaitEvent(ctx, t, events, "output", func(event Event) bool { return event.Category == "stdout" })
	require.Equal(t, "value 7\n", output.Output)
	awaitEvent(ctx, t, events, "terminated")
}

func TestCloseTerminatesStoppedDebuggee(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("probing a process with signal 0 needs Unix")
	}
	ctx, session, events, _ := startInstalled(t)
	process := awaitEvent(ctx, t, events, "process")
	require.Positive(t, process.ProcessID)
	awaitEvent(ctx, t, events, "stopped")
	require.NoError(t, session.Close())
	debuggee, err := os.FindProcess(process.ProcessID)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return debuggee.Signal(syscall.Signal(0)) != nil
	}, 5*time.Second, 20*time.Millisecond, "debuggee outlived Close")
}

func awaitEvent(ctx context.Context, t *testing.T, events <-chan Event, name string, match ...func(Event) bool) Event {
	t.Helper()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatalf("events closed before %s", name)
			}
			if event.Name == name && (len(match) == 0 || match[0](event)) {
				return event
			}
		case <-ctx.Done():
			t.Fatalf("no %s event: %v", name, errors.Unwrap(ctx.Err()))
		}
	}
}
