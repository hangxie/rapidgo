package dap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypedRequests(t *testing.T) {
	for _, test := range []struct {
		name      string
		call      func(context.Context, *Session) (any, error)
		command   string
		arguments string
		body      any
		want      any
	}{
		{
			name: "launch test",
			call: func(ctx context.Context, s *Session) (any, error) {
				return nil, s.Launch(ctx, LaunchConfig{Mode: ModeTest, Program: "/p/pkg", Args: []string{"-test.run", "^TestX$"}, Cwd: "/p/pkg", BuildFlags: []string{"-tags=it"}})
			},
			command:   "launch",
			arguments: `{"mode":"test","program":"/p/pkg","args":["-test.run","^TestX$"],"cwd":"/p/pkg","buildFlags":["-tags=it"],"outputMode":"remote"}`,
		},
		{
			name: "set breakpoints",
			call: func(ctx context.Context, s *Session) (any, error) {
				return s.SetBreakpoints(ctx, "/p/main.go", []SourceBreakpoint{{Line: 4}, {Line: 9, Condition: "i > 2", HitCondition: "3"}})
			},
			command:   "setBreakpoints",
			arguments: `{"breakpoints":[{"line":4},{"line":9,"condition":"i > 2","hitCondition":"3"}],"source":{"path":"/p/main.go"}}`,
			body:      map[string]any{"breakpoints": []map[string]any{{"id": 1, "verified": true, "line": 4, "source": map[string]string{"path": "/p/main.go"}}, {"id": 2, "verified": false, "line": 9, "message": "no code"}}},
			want:      []Breakpoint{{ID: 1, Verified: true, Line: 4, Source: Source{Path: "/p/main.go"}}, {ID: 2, Line: 9, Message: "no code"}},
		},
		{
			name:      "clear breakpoints",
			call:      func(ctx context.Context, s *Session) (any, error) { return s.SetBreakpoints(ctx, "/p/main.go", nil) },
			command:   "setBreakpoints",
			arguments: `{"breakpoints":[],"source":{"path":"/p/main.go"}}`,
			body:      map[string]any{"breakpoints": []any{}},
			want:      []Breakpoint{},
		},
		{
			name:    "configuration done",
			call:    func(ctx context.Context, s *Session) (any, error) { return nil, s.ConfigurationDone(ctx) },
			command: "configurationDone",
		},
		{
			name:      "continue",
			call:      func(ctx context.Context, s *Session) (any, error) { return nil, s.Continue(ctx, 5) },
			command:   "continue",
			arguments: `{"threadId":5}`,
		},
		{
			name:      "next",
			call:      func(ctx context.Context, s *Session) (any, error) { return nil, s.Next(ctx, 5) },
			command:   "next",
			arguments: `{"threadId":5}`,
		},
		{
			name:      "step in",
			call:      func(ctx context.Context, s *Session) (any, error) { return nil, s.StepIn(ctx, 5) },
			command:   "stepIn",
			arguments: `{"threadId":5}`,
		},
		{
			name:      "step out",
			call:      func(ctx context.Context, s *Session) (any, error) { return nil, s.StepOut(ctx, 5) },
			command:   "stepOut",
			arguments: `{"threadId":5}`,
		},
		{
			name:      "pause",
			call:      func(ctx context.Context, s *Session) (any, error) { return nil, s.Pause(ctx, 5) },
			command:   "pause",
			arguments: `{"threadId":5}`,
		},
		{
			name:    "threads",
			call:    func(ctx context.Context, s *Session) (any, error) { return s.Threads(ctx) },
			command: "threads",
			body:    map[string]any{"threads": []map[string]any{{"id": 1, "name": "* [Go 1] main.main"}}},
			want:    []Thread{{ID: 1, Name: "* [Go 1] main.main"}},
		},
		{
			name: "stack trace",
			call: func(ctx context.Context, s *Session) (any, error) {
				frames, total, err := s.StackTrace(ctx, 1, 0, 20)
				return []any{frames, total}, err
			},
			command:   "stackTrace",
			arguments: `{"levels":20,"startFrame":0,"threadId":1}`,
			body:      map[string]any{"stackFrames": []map[string]any{{"id": 1000, "name": "main.main", "line": 11, "column": 0, "source": map[string]string{"name": "main.go", "path": "/p/main.go"}}}, "totalFrames": 3},
			want:      []any{[]StackFrame{{ID: 1000, Name: "main.main", Line: 11, Source: Source{Name: "main.go", Path: "/p/main.go"}}}, 3},
		},
		{
			name:      "scopes",
			call:      func(ctx context.Context, s *Session) (any, error) { return s.Scopes(ctx, 1000) },
			command:   "scopes",
			arguments: `{"frameId":1000}`,
			body:      map[string]any{"scopes": []map[string]any{{"name": "Locals", "variablesReference": 1000}}},
			want:      []Scope{{Name: "Locals", Reference: 1000}},
		},
		{
			name:      "variables",
			call:      func(ctx context.Context, s *Session) (any, error) { return s.Variables(ctx, 1000, 10, 50) },
			command:   "variables",
			arguments: `{"count":50,"start":10,"variablesReference":1000}`,
			body:      map[string]any{"variables": []map[string]any{{"name": "名前", "value": `"gopher"`, "type": "string", "evaluateName": "名前"}, {"name": "s", "value": "[]int len: 3", "variablesReference": 1001, "indexedVariables": 3}}},
			want:      []Variable{{Name: "名前", Value: `"gopher"`, Type: "string", EvaluateName: "名前"}, {Name: "s", Value: "[]int len: 3", Reference: 1001, Indexed: 3}},
		},
		{
			name: "evaluate",
			call: func(ctx context.Context, s *Session) (any, error) {
				return s.Evaluate(ctx, "x*2", 1000, "watch")
			},
			command:   "evaluate",
			arguments: `{"context":"watch","expression":"x*2","frameId":1000}`,
			body:      map[string]any{"result": "84", "type": "int", "variablesReference": 0},
			want:      Variable{Name: "x*2", Value: "84", Type: "int", EvaluateName: "x*2"},
		},
		{
			name: "set variable",
			call: func(ctx context.Context, s *Session) (any, error) {
				return s.SetVariable(ctx, 1000, "x", "7")
			},
			command:   "setVariable",
			arguments: `{"name":"x","value":"7","variablesReference":1000}`,
			body:      map[string]any{"value": "7", "type": "int"},
			want:      Variable{Name: "x", Value: "7", Type: "int"},
		},
		{
			name:      "disconnect",
			call:      func(ctx context.Context, s *Session) (any, error) { return nil, s.Disconnect(ctx, true) },
			command:   "disconnect",
			arguments: `{"terminateDebuggee":true}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, adapter := connectFake(t)
			type outcome struct {
				value any
				err   error
			}
			results := make(chan outcome, 1)
			go func() {
				value, err := test.call(context.Background(), session)
				results <- outcome{value, err}
			}()
			request := adapter.expect(test.command)
			if test.arguments == "" {
				assert.Empty(t, request.Arguments)
			} else {
				assert.JSONEq(t, test.arguments, string(request.Arguments))
			}
			adapter.respond(request, test.body)
			result := <-results
			require.NoError(t, result.err)
			if test.want != nil {
				assert.Equal(t, test.want, result.value)
			}
		})
	}
}

func TestTypedRequestErrors(t *testing.T) {
	session, adapter := connectFake(t)
	errs := make(chan error, 1)
	go func() {
		_, err := session.Evaluate(context.Background(), "missing", 1000, "hover")
		errs <- err
	}()
	adapter.fail(adapter.expect("evaluate"), "could not find symbol value for missing")
	require.EqualError(t, <-errs, "debug adapter evaluate: could not find symbol value for missing")
	go func() {
		_, err := session.SetVariable(context.Background(), 1000, "x", "oops")
		errs <- err
	}()
	adapter.fail(adapter.expect("setVariable"), "bad value")
	require.EqualError(t, <-errs, "debug adapter setVariable: bad value")
	go func() {
		_, err := session.Threads(context.Background())
		errs <- err
	}()
	request := adapter.expect("threads")
	adapter.send(message{Type: "response", RequestSeq: request.Seq, Command: request.Command, Success: true, Body: []byte(`{"threads":"x"}`)})
	require.ErrorContains(t, <-errs, "decode debug adapter threads")
}
