package runcoil

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/runcoil/runcoil/sdks/go/internal/runstate"
)

// Handler is the code of one Task. Build it with HandlerFunc.
type Handler struct {
	call func(ctx context.Context, input []byte) ([]byte, error)
}

// HandlerFunc adapts a typed function to a Handler. The input is decoded from JSON; an
// empty input decodes to the zero value of In. The output is encoded to JSON when fn
// returns no error.
func HandlerFunc[In, Out any](fn func(context.Context, In) (Out, error)) Handler {
	if fn == nil {
		return Handler{}
	}

	return Handler{call: func(ctx context.Context, raw []byte) ([]byte, error) {
		var in In
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, fmt.Errorf("decode input: %w", err)
			}
		}

		out, err := fn(ctx, in)
		if err != nil {
			return nil, err
		}

		b, err := json.Marshal(out)
		if err != nil {
			return nil, &encodeError{err: err}
		}

		return b, nil
	}}
}

// encodeError marks an output that could not be encoded to JSON.
type encodeError struct {
	err error
}

func (e *encodeError) Error() string { return "encode output: " + e.err.Error() }
func (e *encodeError) Unwrap() error { return e.err }

// RunInfo describes the Run that a handler is working on.
type RunInfo struct {
	RunID    string
	App      string
	TaskName string
}

// Info returns the Run that ctx belongs to. It reports false outside a handler.
func Info(ctx context.Context) (RunInfo, bool) {
	s, ok := runstate.From(ctx)
	if !ok {
		return RunInfo{}, false
	}

	return RunInfo{RunID: s.RunID, App: s.App, TaskName: s.TaskName}, true
}
