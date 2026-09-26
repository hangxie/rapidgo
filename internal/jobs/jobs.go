// Package jobs runs cancellable Go commands and streams their output as events.
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

// Request is one command to run, with Target naming the package run needs.
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
	// Pending means requested but not started, often awaiting a cancellation.
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
	// Detected reports locating the go executable in Tool and Err; ID is zero.
	Detected EventType = iota
	// Discovered reports runnable packages in Packages or the failure in Err.
	Discovered
	// Started reports that a job's process is running. Command is set.
	Started
	// Output carries one line of job output in Line and Stream.
	Output
	// Finished reports a terminal State and, when the job failed, Err.
	Finished
)

// Event is one in-order observation about a job, ending in one Finished.
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
