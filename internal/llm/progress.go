package llm

import (
	"context"
	"encoding/json"
	"time"
)

// progressEvery is how often a waiting call reports that it is still running.
const progressEvery = 20 * time.Second

// WithProgress wraps a provider so that long model calls report "still waiting"
// messages through logf, and the duration when they finish. Without it a slow
// call is indistinguishable from a hung one.
func WithProgress(p Provider, logf func(format string, args ...any)) Provider {
	if logf == nil {
		return p
	}
	return &progress{Provider: p, logf: logf, every: progressEvery}
}

type progress struct {
	Provider
	logf  func(format string, args ...any)
	every time.Duration
}

func (p *progress) GenerateJSON(ctx context.Context, req Request) (json.RawMessage, error) {
	start := time.Now()
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		t := time.NewTicker(p.every)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				p.logf("  still waiting for %s (%s)...", p.Name(), time.Since(start).Round(time.Second))
			}
		}
	}()
	out, err := p.Provider.GenerateJSON(ctx, req)
	close(done)
	<-stopped
	if elapsed := time.Since(start); elapsed >= p.every {
		p.logf("  %s answered after %s", p.Name(), elapsed.Round(time.Second))
	}
	return out, err
}
