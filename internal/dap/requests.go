package dap

import "context"

// Launch modes understood by Delve.
const (
	ModeDebug = "debug" // build and debug a main package
	ModeTest  = "test"  // build and debug a package's test binary
)

// LaunchConfig describes the program Delve builds and starts.
type LaunchConfig struct {
	Mode       string            `json:"mode"`
	Program    string            `json:"program"`
	Args       []string          `json:"args,omitempty"`
	Cwd        string            `json:"cwd,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	BuildFlags []string          `json:"buildFlags,omitempty"`
	OutputMode string            `json:"outputMode"`
}

// Source identifies a file known to the adapter.
type Source struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

// SourceBreakpoint is a requested line breakpoint with optional conditions.
type SourceBreakpoint struct {
	Line         int    `json:"line"`
	Condition    string `json:"condition,omitempty"`
	HitCondition string `json:"hitCondition,omitempty"`
}

// Breakpoint is the adapter's view of a requested breakpoint.
type Breakpoint struct {
	ID       int    `json:"id"`
	Verified bool   `json:"verified"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
	Source   Source `json:"source"`
}

// Thread is a Delve goroutine; its ID is the goroutine ID.
type Thread struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// StackFrame is one call-stack entry with one-based line and column.
type StackFrame struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Source Source `json:"source"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Scope groups the variables of a frame, such as Locals.
type Scope struct {
	Name      string `json:"name"`
	Reference int    `json:"variablesReference"`
	Expensive bool   `json:"expensive"`
}

// Variable is a value whose children load through a nonzero Reference.
type Variable struct {
	Name         string `json:"name"`
	Value        string `json:"value"`
	Type         string `json:"type"`
	EvaluateName string `json:"evaluateName"`
	Reference    int    `json:"variablesReference"`
	Named        int    `json:"namedVariables"`
	Indexed      int    `json:"indexedVariables"`
}

// Launch builds and starts the debuggee, routing its output through output events.
func (s *Session) Launch(ctx context.Context, config LaunchConfig) error {
	config.OutputMode = "remote"
	return s.request(ctx, "launch", config, nil)
}

// SetBreakpoints replaces every line breakpoint in path.
func (s *Session) SetBreakpoints(ctx context.Context, path string, requested []SourceBreakpoint) ([]Breakpoint, error) {
	if requested == nil {
		requested = []SourceBreakpoint{}
	}
	var body struct {
		Breakpoints []Breakpoint `json:"breakpoints"`
	}
	arguments := map[string]any{"source": Source{Path: path}, "breakpoints": requested}
	err := s.request(ctx, "setBreakpoints", arguments, &body)
	return body.Breakpoints, err
}

// ConfigurationDone lets the debuggee run once breakpoints are set.
func (s *Session) ConfigurationDone(ctx context.Context) error {
	return s.request(ctx, "configurationDone", nil, nil)
}

// Continue resumes every goroutine.
func (s *Session) Continue(ctx context.Context, threadID int) error {
	return s.request(ctx, "continue", map[string]int{"threadId": threadID}, nil)
}

// Next steps over the current line of threadID.
func (s *Session) Next(ctx context.Context, threadID int) error {
	return s.request(ctx, "next", map[string]int{"threadId": threadID}, nil)
}

// StepIn steps into the call on the current line of threadID.
func (s *Session) StepIn(ctx context.Context, threadID int) error {
	return s.request(ctx, "stepIn", map[string]int{"threadId": threadID}, nil)
}

// StepOut runs threadID until its current function returns.
func (s *Session) StepOut(ctx context.Context, threadID int) error {
	return s.request(ctx, "stepOut", map[string]int{"threadId": threadID}, nil)
}

// Pause halts a running debuggee.
func (s *Session) Pause(ctx context.Context, threadID int) error {
	return s.request(ctx, "pause", map[string]int{"threadId": threadID}, nil)
}

// Threads lists the debuggee's goroutines.
func (s *Session) Threads(ctx context.Context) ([]Thread, error) {
	var body struct {
		Threads []Thread `json:"threads"`
	}
	err := s.request(ctx, "threads", nil, &body)
	return body.Threads, err
}

// StackTrace loads up to levels frames of threadID starting at start, plus the total depth when known.
func (s *Session) StackTrace(ctx context.Context, threadID, start, levels int) ([]StackFrame, int, error) {
	var body struct {
		Frames []StackFrame `json:"stackFrames"`
		Total  int          `json:"totalFrames"`
	}
	arguments := map[string]int{"threadId": threadID, "startFrame": start, "levels": levels}
	err := s.request(ctx, "stackTrace", arguments, &body)
	return body.Frames, body.Total, err
}

// Scopes lists the variable groups of frameID.
func (s *Session) Scopes(ctx context.Context, frameID int) ([]Scope, error) {
	var body struct {
		Scopes []Scope `json:"scopes"`
	}
	err := s.request(ctx, "scopes", map[string]int{"frameId": frameID}, &body)
	return body.Scopes, err
}

// Variables loads up to count children of reference starting at start; zero count loads all.
func (s *Session) Variables(ctx context.Context, reference, start, count int) ([]Variable, error) {
	var body struct {
		Variables []Variable `json:"variables"`
	}
	arguments := map[string]int{"variablesReference": reference, "start": start, "count": count}
	err := s.request(ctx, "variables", arguments, &body)
	return body.Variables, err
}

// Evaluate computes expression in frameID; purpose is "watch", "hover", or "repl".
func (s *Session) Evaluate(ctx context.Context, expression string, frameID int, purpose string) (Variable, error) {
	var body struct {
		Result    string `json:"result"`
		Type      string `json:"type"`
		Reference int    `json:"variablesReference"`
		Named     int    `json:"namedVariables"`
		Indexed   int    `json:"indexedVariables"`
	}
	arguments := map[string]any{"expression": expression, "frameId": frameID, "context": purpose}
	if err := s.request(ctx, "evaluate", arguments, &body); err != nil {
		return Variable{}, err
	}
	return Variable{Name: expression, Value: body.Result, Type: body.Type, EvaluateName: expression, Reference: body.Reference, Named: body.Named, Indexed: body.Indexed}, nil
}

// SetVariable assigns value to the child name of reference and returns the stored value.
func (s *Session) SetVariable(ctx context.Context, reference int, name, value string) (Variable, error) {
	var body struct {
		Value     string `json:"value"`
		Type      string `json:"type"`
		Reference int    `json:"variablesReference"`
	}
	arguments := map[string]any{"variablesReference": reference, "name": name, "value": value}
	if err := s.request(ctx, "setVariable", arguments, &body); err != nil {
		return Variable{}, err
	}
	return Variable{Name: name, Value: body.Value, Type: body.Type, Reference: body.Reference}, nil
}

// Disconnect ends the debug session, terminating the debuggee when terminate is set.
func (s *Session) Disconnect(ctx context.Context, terminate bool) error {
	return s.request(ctx, "disconnect", map[string]bool{"terminateDebuggee": terminate}, nil)
}
