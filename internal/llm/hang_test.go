package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/genai"
)

// A server that accepts the request and never answers must not hang the tool:
// each attempt times out, is retried, and the call finally fails.
func TestGeminiHangingServerTimesOut(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body) // lets the server notice a client that gives up
		select {                           // never reply
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "k",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	g := fast(&Gemini{client: client, model: "m", timeout: 60 * time.Millisecond}, geminiRetry)

	done := make(chan error, 1)
	go func() {
		_, err := g.GenerateJSON(context.Background(), Request{System: "s", User: "u", Schema: json.RawMessage(`{"type":"object"}`)})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("GenerateJSON hung despite the per-attempt timeout")
	}
	if calls.Load() != int32(geminiRetry.Attempts) {
		t.Fatalf("attempts = %d, want %d", calls.Load(), geminiRetry.Attempts)
	}
}

func TestProgressReportsSlowCalls(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	logf := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, strings.TrimSpace(format))
	}
	slow := NewFake(func(Request) (json.RawMessage, error) {
		time.Sleep(120 * time.Millisecond)
		return json.RawMessage(`{}`), nil
	})
	p := WithProgress(slow, logf).(*progress)
	p.every = 30 * time.Millisecond

	if _, err := p.GenerateJSON(context.Background(), Request{}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	waiting, answered := 0, 0
	for _, l := range lines {
		if strings.Contains(l, "waiting") {
			waiting++
		}
		if strings.Contains(l, "answered after") {
			answered++
		}
	}
	if waiting < 2 || answered != 1 {
		t.Fatalf("waiting=%d answered=%d lines=%v", waiting, answered, lines)
	}
}

func TestProgressSilentForFastCalls(t *testing.T) {
	var n atomic.Int32
	p := WithProgress(NewFake(Reply(map[string]int{"a": 1})), func(string, ...any) { n.Add(1) })
	if _, err := p.GenerateJSON(context.Background(), Request{}); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 0 {
		t.Fatal("fast calls should not print anything")
	}
	if p.Name() != "fake" {
		t.Fatalf("Name() = %q", p.Name())
	}
}
