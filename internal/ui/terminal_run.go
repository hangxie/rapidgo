package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/gdamore/tcell/v2"

	"github.com/hangxie/rapidgo/internal/i18n"
	"github.com/hangxie/rapidgo/internal/jobs"
)

// screenEventPump owns one screen event channel until terminal handoff.
type screenEventPump struct {
	events chan tcell.Event
	stopQ  chan struct{}
	done   chan struct{}
}

func startScreenEvents(screen tcell.Screen) *screenEventPump {
	pump := &screenEventPump{
		events: make(chan tcell.Event, 1),
		stopQ:  make(chan struct{}),
		done:   make(chan struct{}),
	}
	go func() {
		screen.ChannelEvents(pump.events, pump.stopQ)
		close(pump.done)
	}()
	return pump
}

func (pump *screenEventPump) stop() {
	close(pump.stopQ)
	<-pump.done
}

// handoffTerminal pauses the UI, runs the child, and restores rendering.
func handoffTerminal(ctx context.Context, screen tcell.Screen, manager *jobs.Manager, state *shellState, interrupts <-chan os.Signal, pump **screenEventPump) error {
	request := *state.terminalRun
	state.terminalRun = nil
	if manager.Running(jobs.Build) || manager.Running(jobs.Test) || manager.Running(jobs.Run) {
		state.message = i18n.Text("msg_stop_other_go_jobs_before_running_in_the_terminal")
		render(screen, *state)
		return nil
	}
	var runErr error
	lend := func() { runErr = runAttachedWithPrompt(ctx, manager, request, interrupts) }
	if err := withSuspendedScreen(screen, pump, lend); err != nil {
		return err
	}
	state.message = i18n.Format("msg_terminal_run_s_finished", request.Command())
	if runErr != nil {
		state.message = i18n.Format("msg_terminal_run_s_failed_s", request.Command(), runErr.Error())
	}
	screen.Sync()
	render(screen, *state)
	return nil
}

// withSuspendedScreen lends the terminal to child and resumes rendering after.
func withSuspendedScreen(screen tcell.Screen, pump **screenEventPump, child func()) error {
	(*pump).stop()
	*pump = nil
	// Suspend rather than Fini: tcell only ever finishes a screen once, so
	// finishing here would leave nothing to restore the terminal on quit.
	if err := screen.Suspend(); err != nil {
		return i18n.Errorf("msg_suspend_terminal_screen_w", err)
	}
	child()
	if err := screen.Resume(); err != nil {
		return i18n.Errorf("msg_restore_terminal_screen_w", err)
	}
	*pump = startScreenEvents(screen)
	return nil
}

// runAttachedWithPrompt lends the restored terminal to a child program.
func runAttachedWithPrompt(parent context.Context, manager *jobs.Manager, request jobs.Request, interrupts <-chan os.Signal) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	done := make(chan struct{})
	interrupted := make(chan struct{}, 1)
	go func() {
		select {
		case <-interrupts:
			interrupted <- struct{}{}
			cancel()
		case <-done:
		}
	}()
	err := manager.RunAttached(ctx, request, os.Stdin, os.Stdout, os.Stderr)
	close(done)
	select {
	case <-interrupted:
		return err
	default:
	}
	if err != nil {
		_, _ = fmt.Fprint(os.Stdout, i18n.Format("msg_s_failed_v", request.Command(), err))
	} else {
		_, _ = fmt.Fprint(os.Stdout, i18n.Format("msg_s_finished", request.Command()))
	}
	_, _ = fmt.Fprint(os.Stdout, i18n.Text("msg_press_enter_to_return_to_rapidgo"))
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	return err
}
