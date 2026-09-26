// Package jobs runs cancellable Go toolchain commands and streams their output.
//
// It has no terminal dependency: callers start jobs, read typed events from a
// channel, and decide how to display them.
package jobs

import "strings"

// Kind identifies one of the Go commands RapidGo can run.
type Kind uint8

const (
	Build Kind = iota
	Test
	Run
	kindCount
)

func (k Kind) String() string {
	switch k {
	case Build:
		return "build"
	case Test:
		return "test"
	case Run:
		return "run"
	}
	return "unknown"
}

// Request is one command to run. Build and test always cover the whole module;
// run needs a single main package, which the caller resolves and passes as
// Target. An empty Target means the project root.
type Request struct {
	Kind   Kind
	Target string
}

// Args returns the arguments passed to the go executable.
func (r Request) Args() []string {
	switch r.Kind {
	case Build:
		return []string{"build", "./..."}
	case Test:
		return []string{"test", "./..."}
	case Run:
		return []string{"run", r.target()}
	}
	return nil
}

// Command renders the command line for display only.
func (r Request) Command() string {
	args := r.Args()
	if len(args) == 0 {
		return ""
	}
	return "go " + strings.Join(args, " ")
}

func (r Request) target() string {
	if r.Target == "" {
		return "."
	}
	return r.Target
}

// State is the lifecycle state of a single job.
type State uint8

const (
	// Pending means the job was requested but has not started yet, usually
	// because an earlier job of the same kind is still being cancelled.
	Pending State = iota
	Running
	Succeeded
	Failed
	Cancelled
)

func (s State) String() string {
	switch s {
	case Pending:
		return "pending"
	case Running:
		return "running"
	case Succeeded:
		return "succeeded"
	case Failed:
		return "failed"
	case Cancelled:
		return "cancelled"
	}
	return "unknown"
}

// Done reports whether the state is terminal.
func (s State) Done() bool { return s == Succeeded || s == Failed || s == Cancelled }

// Stream distinguishes the two output streams of a job.
type Stream uint8

const (
	Stdout Stream = iota
	Stderr
)

func (s Stream) String() string {
	if s == Stderr {
		return "stderr"
	}
	return "stdout"
}

// EventType describes which fields of an Event are meaningful.
type EventType uint8

const (
	// Detected reports the result of locating the go executable. Its Tool and
	// Err fields are set and its ID is zero.
	Detected EventType = iota
	// Discovered reports the module's runnable packages in Packages, or why
	// they could not be listed in Err. Its ID is zero.
	Discovered
	// Started reports that a job's process is running. Command is set.
	Started
	// Output carries one line of job output in Line and Stream.
	Output
	// Finished reports a terminal State and, when the job failed, Err.
	Finished
)

// Event is a single observation about a job. Events of one job always arrive
// in order, and every job emits exactly one Finished event.
type Event struct {
	ID       uint64
	Kind     Kind
	Type     EventType
	Tool     Toolchain
	Packages []Package
	Command  string
	Line     string
	Stream   Stream
	State    State
	Err      error
}
