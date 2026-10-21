package debug

import (
	"context"
	"slices"

	"github.com/hangxie/rapidgo/internal/dap"
)

// Continue resumes a paused program; a failure arrives as a Problem event.
func (s *Session) Continue(threadID int) {
	s.step(Continue, threadID, Adapter.Continue)
}

// Next steps over the current line of a paused thread.
func (s *Session) Next(threadID int) { s.step(Next, threadID, Adapter.Next) }

// StepIn steps into the call on the current line of a paused thread.
func (s *Session) StepIn(threadID int) { s.step(StepIn, threadID, Adapter.StepIn) }

// StepOut runs a paused thread until its current function returns.
func (s *Session) StepOut(threadID int) { s.step(StepOut, threadID, Adapter.StepOut) }

// SetBreakpoints replaces one file's breakpoints in the running program; the answer arrives as a Resolved event.
func (s *Session) SetBreakpoints(request Request) {
	s.mu.Lock()
	if !s.configured && !s.closing {
		s.pending = queue(s.pending, request)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.command(func(ctx context.Context, adapter Adapter) error {
		_, err := s.resolve(ctx, adapter, request)
		return err
	}, func(err error) Event {
		return Event{Kind: Resolved, Request: request, Err: err}
	})
}

// RunTo sets request, which RunTo on Breakpoints builds, and continues; the caller restores the file once the program stops.
func (s *Session) RunTo(request Request, threadID int) {
	s.command(func(ctx context.Context, adapter Adapter) error {
		results, err := s.resolve(ctx, adapter, request)
		if err != nil {
			return err
		}
		// A line Delve cannot stop on, such as a blank one, would let the program run past it, perhaps to its end.
		if err := request.stopRefused(results); err != nil {
			return err
		}
		return adapter.Continue(ctx, threadID)
	}, problem(RunTo, threadID))
}

// Locate loads the frame a paused thread stopped in; it arrives as a Located event.
func (s *Session) Locate(threadID int) {
	s.command(func(ctx context.Context, adapter Adapter) error {
		frames, _, err := adapter.StackTrace(ctx, threadID, 0, 1)
		if err == nil {
			s.send(Event{Kind: Located, ThreadID: threadID, Frames: frames})
		}
		return err
	}, problem(Locate, threadID))
}

func (s *Session) step(command Command, threadID int, call func(Adapter, context.Context, int) error) {
	s.command(func(ctx context.Context, adapter Adapter) error {
		return call(adapter, ctx, threadID)
	}, problem(command, threadID))
}

func (s *Session) resolve(ctx context.Context, adapter Adapter, request Request) ([]dap.Breakpoint, error) {
	results, err := adapter.SetBreakpoints(ctx, request.Path, request.Breakpoints)
	if err == nil {
		s.send(Event{Kind: Resolved, Request: request, Results: results})
	}
	return results, err
}

// queue adds request to requests, replacing an older one for the same file.
func queue(requests []Request, request Request) []Request {
	if index := slices.IndexFunc(requests, func(queued Request) bool { return queued.Path == request.Path }); index >= 0 {
		requests[index] = request
		return requests
	}
	return append(requests, request)
}

func problem(command Command, threadID int) func(error) Event {
	return func(err error) Event {
		return Event{Kind: Problem, Command: command, ThreadID: threadID, Err: err}
	}
}

// command runs call in the background after earlier commands, and sends failure's event if it fails.
func (s *Session) command(call func(context.Context, Adapter) error, failure func(error) Event) {
	s.mu.Lock()
	adapter := s.adapter
	if s.closing || adapter == nil {
		s.mu.Unlock()
		return
	}
	s.commands.Add(1)
	// Delve applies requests in the order they arrive, so a step must not overtake the breakpoints before it.
	previous, done := s.last, make(chan struct{})
	s.last = done
	s.mu.Unlock()
	go func() {
		defer s.commands.Done()
		defer close(done)
		if previous != nil {
			<-previous
		}
		err := call(s.ctx, adapter)
		s.mu.Lock()
		report := err != nil && !s.closing && s.ctx.Err() == nil
		s.mu.Unlock()
		if report {
			s.send(failure(err))
		}
	}()
}
