// Package ui owns terminal input, layout, and rendering.
package ui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gdamore/tcell/v2"
)

// Run opens the terminal shell for a project and restores the terminal on exit.
func Run(projectRoot string) error {
	screen, err := tcell.NewScreen()
	if err != nil {
		return fmt.Errorf("create terminal screen: %w", err)
	}
	if err := screen.Init(); err != nil {
		return fmt.Errorf("initialize terminal screen: %w", err)
	}
	defer screen.Fini()

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupts)

	return runLoop(screen, projectRoot, interrupts)
}

func runLoop(screen tcell.Screen, projectRoot string, interrupts <-chan os.Signal) error {
	events := make(chan tcell.Event, 1)
	stopEvents := make(chan struct{})
	eventsDone := make(chan struct{})
	go func() {
		screen.ChannelEvents(events, stopEvents)
		close(eventsDone)
	}()
	defer func() {
		close(stopEvents)
		<-eventsDone
	}()

	state := shellState{projectRoot: projectRoot}
	render(screen, state)
	for {
		// Check cancellation before the next event so a flood of terminal
		// events cannot indefinitely delay a pending signal.
		select {
		case <-interrupts:
			return nil
		default:
		}
		var event tcell.Event
		select {
		case <-interrupts:
			return nil
		case next, ok := <-events:
			if !ok {
				return fmt.Errorf("terminal screen closed")
			}
			event = next
		}
		if event == nil {
			return fmt.Errorf("terminal screen closed")
		}
		if handleEvent(screen, &state, event) {
			return nil
		}
		render(screen, state)
	}
}

type shellState struct {
	projectRoot string
	helpVisible bool
	menuOpen    bool
	menuIndex   int
}

func handleEvent(screen tcell.Screen, state *shellState, event tcell.Event) bool {
	switch event := event.(type) {
	case *tcell.EventKey:
		switch event.Key() {
		case tcell.KeyCtrlQ, tcell.KeyCtrlC:
			return true
		case tcell.KeyF1:
			state.menuOpen = false
			state.helpVisible = !state.helpVisible
		case tcell.KeyF10:
			state.helpVisible = false
			state.menuOpen = !state.menuOpen
		case tcell.KeyEscape:
			state.menuOpen = false
			state.helpVisible = false
		case tcell.KeyLeft:
			if state.menuOpen {
				state.menuIndex = (state.menuIndex + menuCount - 1) % menuCount
			}
		case tcell.KeyRight:
			if state.menuOpen {
				state.menuIndex = (state.menuIndex + 1) % menuCount
			}
		case tcell.KeyEnter:
			if state.menuOpen {
				state.menuOpen = false
				if state.menuIndex == menuFile {
					return true
				}
				state.helpVisible = true
			}
		case tcell.KeyRune:
			if event.Modifiers()&tcell.ModAlt != 0 {
				switch event.Rune() {
				case 'f', 'F':
					state.menuIndex = menuFile
					state.menuOpen = true
					state.helpVisible = false
				case 'h', 'H':
					state.menuIndex = menuHelp
					state.menuOpen = true
					state.helpVisible = false
				}
			}
		}
	case *tcell.EventResize:
		screen.Sync()
	case *tcell.EventInterrupt:
		return true
	}
	return false
}
