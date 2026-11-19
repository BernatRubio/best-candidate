package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/genai"
)

func chatReply(content, finish string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{
			"message":       map[string]string{"content": content},
			"finish_reason": finish,
		}},
	})
	return string(b)
}

func newCompat(t *testing.T, url, mode string) *OpenAICompat {
	t.Helper()
	o, err := NewOpenAICompat(OpenAICompatConfig{Name: "test", BaseURL: url, APIKey: "sk-test", Model: "m", JSONMode: mode})
	if err != nil {
		t.Fatal(err)
	}
	o.backoff = time.Millisecond
	return o
}

var req = Request{System: "be honest", User: "hello", Schema: json.RawMessage(`{"type":"object"}`)}

func TestCompatSchemaMode(t *testing.T) {
	var got map[string]any
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write([]byte(chatReply(`{"ok":true}`, "stop")))
	}))
	defer srv.Close()

	out, err := newCompat(t, srv.URL+"/v1/", JSONModeSchema).GenerateJSON(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"ok":true}` {
		t.Fatalf("out = %s", out)
	}
	if auth != "Bearer sk-test" || path != "/v1/chat/completions" {
		t.Fatalf("auth=%q path=%q", auth, path)
	}
	rf := got["response_format"].(map[string]any)
	if rf["type"] != "json_schema" {
		t.Fatalf("response_format = %v", rf)
	}
	js := rf["json_schema"].(map[string]any)
	if js["strict"] != true || js["schema"].(map[string]any)["type"] != "object" {
		t.Fatalf("json_schema = %v", js)
	}
	msgs := got["messages"].([]any)
	if msgs[0].(map[string]any)["content"] != "be honest" || msgs[1].(map[string]any)["role"] != "user" {
		t.Fatalf("messages = %v", msgs)
	}
}

func TestCompatObjectModeStripsFences(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write([]byte(chatReply("```json\n{\"ok\": true}\n```", "stop")))
	}))
	defer srv.Close()

	out, err := newCompat(t, srv.URL, JSONModeObject).GenerateJSON(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"ok": true}` {
		t.Fatalf("out = %s", out)
	}
	if got["response_format"].(map[string]any)["type"] != "json_object" {
		t.Fatalf("response_format = %v", got["response_format"])
	}
	system := got["messages"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(system, `{"type":"object"}`) {
		t.Fatalf("schema missing from system prompt: %s", system)
	}
}

func TestCompatRetriesTransientErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			http.Error(w, `{"error":{"message":"slow down"}}`, http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(chatReply(`{"ok":true}`, "stop")))
	}))
	defer srv.Close()

	if _, err := newCompat(t, srv.URL, "").GenerateJSON(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestCompatGivesUpAfterRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, `{"error":{"message":"overloaded"}}`, http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := newCompat(t, srv.URL, "").GenerateJSON(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "overloaded") {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != maxAttempts {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestCompatDoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, `{"error":{"message":"response_format unsupported"}}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := newCompat(t, srv.URL, "").GenerateJSON(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "BC_JSON_MODE=object") {
		t.Fatalf("err should hint at BC_JSON_MODE: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestCompatRefusalAndTruncation(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"refusal": {`{"choices":[{"message":{"content":"","refusal":"I can't help"},"finish_reason":"stop"}]}`, "declined"},
		"length":  {chatReply(`{"ok":`, "length"), "cut off"},
		"badjson": {chatReply("sorry, here you go", "stop"), "valid JSON"},
		"empty":   {`{"choices":[]}`, "no choices"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := newCompat(t, srv.URL, "").GenerateJSON(context.Background(), req)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCompatNoAuthHeaderWithoutKey(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(chatReply(`{}`, "stop")))
	}))
	defer srv.Close()
	o, _ := NewOpenAICompat(OpenAICompatConfig{Name: "ollama", BaseURL: srv.URL, Model: "llama"})
	if _, err := o.GenerateJSON(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		t.Fatalf("unexpected Authorization header %q", auth)
	}
}

func TestExtractJSON(t *testing.T) {
	for in, want := range map[string]string{
		`{"a":1}`:                              `{"a":1}`,
		"```json\n{\"a\":1}\n```":              `{"a":1}`,
		"```\n{\"a\":1}\n```":                  `{"a":1}`,
		"Here you go: {\"a\":1} Hope it helps": `{"a":1}`,
	} {
		got, err := ExtractJSON(in)
		if err != nil || string(got) != want {
			t.Errorf("ExtractJSON(%q) = %s, %v", in, got, err)
		}
	}
	if _, err := ExtractJSON("no json here"); err == nil {
		t.Error("expected error")
	}
}

func TestNewValidation(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("BC_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("BC_JSON_MODE", "")
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"unknown", Config{Provider: "foo", Model: "m"}, "unknown provider"},
		{"openai needs model", Config{Provider: "openai"}, "needs a model"},
		{"openai needs key", Config{Provider: "openai", Model: "m"}, "OPENAI_API_KEY"},
		{"gemini needs key", Config{Provider: "gemini", Model: "m"}, "GEMINI_API_KEY"},
		{"compat needs url", Config{Provider: "openai-compatible", Model: "m"}, "base URL"},
		{"ollama needs model", Config{Provider: "ollama"}, "needs a model"},
	}
	for _, tc := range cases {
		_, err := New(tc.cfg)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}

	p, err := New(Config{Provider: "ollama", Model: "llama3"})
	if err != nil || p.Name() != "ollama/llama3" {
		t.Fatalf("ollama: %v %v", p, err)
	}
	t.Setenv("OPENAI_API_KEY", "sk")
	if p, err := New(Config{Provider: "openai", Model: "m"}); err != nil || p.Name() != "openai/m" {
		t.Fatalf("openai: %v %v", p, err)
	}
	t.Setenv("BC_JSON_MODE", "weird")
	if _, err := New(Config{Provider: "ollama", Model: "m"}); err == nil {
		t.Fatal("expected error for bad BC_JSON_MODE")
	}
}

func TestFromEnvOverridesFlags(t *testing.T) {
	t.Setenv("BC_PROVIDER", "ollama")
	t.Setenv("BC_MODEL", "qwen")
	t.Setenv("BC_BASE_URL", "")
	t.Setenv("BC_JSON_MODE", "")
	p, err := FromEnv(Config{Provider: "anthropic", Model: "ignored"})
	if err != nil || p.Name() != "ollama/qwen" {
		t.Fatalf("%v %v", p, err)
	}
}

func TestRetryTransientThenSuccess(t *testing.T) {
	calls := 0
	transient := func(err error) bool { return strings.Contains(err.Error(), "busy") }
	var waits []time.Duration
	out, err := retry(context.Background(), 5, time.Millisecond, transient,
		func(_ error, w time.Duration) { waits = append(waits, w) },
		func() (string, error) {
			calls++
			if calls < 3 {
				return "", errors.New("busy")
			}
			return "ok", nil
		})
	if err != nil || out != "ok" || calls != 3 {
		t.Fatalf("out=%q err=%v calls=%d", out, err, calls)
	}
	if len(waits) != 2 || waits[1] != 2*waits[0] {
		t.Fatalf("backoff should double: %v", waits)
	}
}

func TestRetryStopsOnPermanentErrorAndAfterAttempts(t *testing.T) {
	transient := func(err error) bool { return err.Error() == "busy" }

	calls := 0
	_, err := retry(context.Background(), 5, time.Millisecond, transient, nil, func() (int, error) {
		calls++
		return 0, errors.New("bad request")
	})
	if err == nil || calls != 1 {
		t.Fatalf("permanent error must not be retried: err=%v calls=%d", err, calls)
	}

	calls = 0
	_, err = retry(context.Background(), 3, time.Millisecond, transient, nil, func() (int, error) {
		calls++
		return 0, errors.New("busy")
	})
	if err == nil || calls != 3 {
		t.Fatalf("should give up after 3 attempts: err=%v calls=%d", err, calls)
	}
}

func TestRetryHonoursContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := retry(ctx, 5, time.Hour, func(error) bool { return true }, nil, func() (int, error) {
		calls++
		cancel()
		return 0, errors.New("busy")
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestIsTransientGemini(t *testing.T) {
	for code, want := range map[int]bool{429: true, 500: true, 503: true, 504: true, 400: false, 401: false, 404: false} {
		err := fmt.Errorf("wrapped: %w", genai.APIError{Code: code, Status: "X"})
		if got := isTransientGemini(err); got != want {
			t.Errorf("code %d: got %v want %v", code, got, want)
		}
	}
	if isTransientGemini(errors.New("plain")) {
		t.Error("non-API errors are not transient")
	}
}

func TestAPIMessageShowsUpstreamDetail(t *testing.T) {
	body := `{"error":{"message":"Provider returned error","code":429,"metadata":{"raw":"model is temporarily rate-limited upstream","provider_name":"SomeHost"}}}`
	got := apiMessage([]byte(body))
	for _, want := range []string{"Provider returned error", "SomeHost", "rate-limited upstream"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if got := apiMessage([]byte(`{"error":{"message":"plain"}}`)); got != "plain" {
		t.Errorf("plain error changed: %q", got)
	}
	if got := apiMessage([]byte(`{"error":{"message":"m","metadata":{"raw":{"x":1}}}}`)); !strings.Contains(got, `{"x":1}`) {
		t.Errorf("object raw not shown: %q", got)
	}
	if got := apiMessage([]byte("<html>bad gateway</html>")); !strings.Contains(got, "bad gateway") {
		t.Errorf("non-JSON body lost: %q", got)
	}
}
