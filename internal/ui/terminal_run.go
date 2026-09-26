package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/gdamore/tcell/v2"

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
		state.message = "Stop other Go jobs before running in the terminal"
		render(screen, *state)
		return nil
	}
	(*pump).stop()
	*pump = nil
	screen.Fini()
	runErr := runAttachedWithPrompt(ctx, manager, request, interrupts)
	if err := screen.Init(); err != nil {
		return fmt.Errorf("restore terminal screen: %w", err)
	}
	*pump = startScreenEvents(screen)
	state.message = "Terminal run " + request.Command() + " finished"
	if runErr != nil {
		state.message = "Terminal run " + request.Command() + " failed: " + runErr.Error()
	}
	screen.Sync()
	render(screen, *state)
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
		_, _ = fmt.Fprintf(os.Stdout, "\n%s failed: %v\n", request.Command(), err)
	} else {
		_, _ = fmt.Fprintf(os.Stdout, "\n%s finished\n", request.Command())
	}
	_, _ = fmt.Fprint(os.Stdout, "Press Enter to return to RapidGo...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	return err
}
