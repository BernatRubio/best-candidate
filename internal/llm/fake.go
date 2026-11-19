package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Fake is a scripted provider for tests. Each call pops the next handler.
type Fake struct {
	mu       sync.Mutex
	handlers []func(Request) (json.RawMessage, error)
	Calls    []Request
}

// NewFake queues one handler per expected call.
func NewFake(handlers ...func(Request) (json.RawMessage, error)) *Fake {
	return &Fake{handlers: handlers}
}

// Reply is a handler that always returns v marshalled as JSON.
func Reply(v any) func(Request) (json.RawMessage, error) {
	return func(Request) (json.RawMessage, error) {
		raw, err := json.Marshal(v)
		return raw, err
	}
}

func (f *Fake) Name() string { return "fake" }

func (f *Fake) GenerateJSON(_ context.Context, req Request) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, req)
	if len(f.handlers) == 0 {
		return nil, fmt.Errorf("fake provider: unexpected call #%d", len(f.Calls))
	}
	h := f.handlers[0]
	f.handlers = f.handlers[1:]
	return h(req)
}
