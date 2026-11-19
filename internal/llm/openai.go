package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// JSON output modes for OpenAI-compatible servers.
const (
	// JSONModeSchema sends the schema as response_format json_schema (strict).
	JSONModeSchema = "schema"
	// JSONModeObject asks only for a JSON object and puts the schema in the
	// prompt, for servers that do not support json_schema.
	JSONModeObject = "object"
)

// OpenAICompatConfig configures a client for the /chat/completions API shared
// by OpenAI, Ollama, OpenRouter, Groq, LM Studio, vLLM and others.
type OpenAICompatConfig struct {
	Name        string
	BaseURL     string
	APIKey      string
	KeyRequired string // when set and APIKey is empty, fail naming this env var
	Model       string
	JSONMode    string
}

// OpenAICompat talks to any OpenAI-compatible chat completions endpoint.
type OpenAICompat struct {
	cfg     OpenAICompatConfig
	http    *http.Client
	backoff time.Duration
}

// NewOpenAICompat validates the configuration and builds the client.
func NewOpenAICompat(cfg OpenAICompatConfig) (*OpenAICompat, error) {
	if cfg.KeyRequired != "" && cfg.APIKey == "" {
		return nil, fmt.Errorf("%s is not set", cfg.KeyRequired)
	}
	switch cfg.JSONMode {
	case "":
		cfg.JSONMode = JSONModeSchema
	case JSONModeSchema, JSONModeObject:
	default:
		return nil, fmt.Errorf("BC_JSON_MODE must be %q or %q, got %q", JSONModeSchema, JSONModeObject, cfg.JSONMode)
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &OpenAICompat{
		cfg:     cfg,
		http:    &http.Client{Timeout: 10 * time.Minute},
		backoff: 2 * time.Second,
	}, nil
}

func (o *OpenAICompat) Name() string { return o.cfg.Name + "/" + o.cfg.Model }

// maxAttempts bounds retries of rate limits and overload: with a 2s base the
// waits are 2s, 4s, 8s and 16s (or the server's Retry-After, up to 30s).
const maxAttempts = 5

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			Refusal string `json:"refusal"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// GenerateJSON requests a JSON document matching req.Schema. Rate limits and
// transient server errors are retried with backoff.
func (o *OpenAICompat) GenerateJSON(ctx context.Context, req Request) (json.RawMessage, error) {
	system := req.System
	var format map[string]any
	if o.cfg.JSONMode == JSONModeObject {
		system += "\n\nRespond with a single JSON object that validates against this JSON schema, and nothing else (no prose, no code fences):\n" + string(req.Schema)
		format = map[string]any{"type": "json_object"}
	} else {
		format = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "output",
				"strict": true,
				"schema": req.Schema,
			},
		}
	}
	body, err := json.Marshal(map[string]any{
		"model": o.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": req.User},
		},
		"response_format": format,
	})
	if err != nil {
		return nil, err
	}

	var lastErr error
	notify := stderrNotify(o.cfg.Name, maxAttempts)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			wait := o.backoff << (attempt - 1)
			if ra, ok := lastErr.(*retryableError); ok && ra.after > 0 {
				wait = ra.after
			}
			notify(lastErr, wait)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		out, err := o.once(ctx, body)
		if err == nil {
			return out, nil
		}
		var re *retryableError
		if !errors.As(err, &re) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

type retryableError struct {
	err   error
	after time.Duration
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

func (o *OpenAICompat) once(ctx context.Context, body []byte) (json.RawMessage, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.cfg.APIKey)
	}

	resp, err := o.http.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &retryableError{err: fmt.Errorf("%s request: %w", o.cfg.Name, err)}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, &retryableError{err: fmt.Errorf("reading %s response: %w", o.cfg.Name, err)}
	}

	if resp.StatusCode != http.StatusOK {
		apiErr := fmt.Errorf("%s API error %d: %s", o.cfg.Name, resp.StatusCode, apiMessage(data))
		switch resp.StatusCode {
		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			re := &retryableError{err: apiErr}
			if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 && secs <= 30 {
				re.after = time.Duration(secs) * time.Second
			}
			return nil, re
		case http.StatusBadRequest:
			return nil, fmt.Errorf("%w (if the server does not support json_schema output, set BC_JSON_MODE=object)", apiErr)
		case http.StatusUnauthorized, http.StatusForbidden:
			if o.cfg.APIKey == "" {
				return nil, fmt.Errorf("%w (no API key was sent: export BC_API_KEY, or OPENAI_API_KEY, in this shell)", apiErr)
			}
			return nil, fmt.Errorf("%w (the API key was rejected: check it and that it belongs to this provider)", apiErr)
		}
		return nil, apiErr
	}

	var cr chatResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, fmt.Errorf("decoding %s response: %w", o.cfg.Name, err)
	}
	if cr.Error != nil {
		return nil, fmt.Errorf("%s error: %s", o.cfg.Name, cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return nil, fmt.Errorf("%s returned no choices", o.cfg.Name)
	}
	c := cr.Choices[0]
	if c.Message.Refusal != "" {
		return nil, fmt.Errorf("the model declined this request: %s", c.Message.Refusal)
	}
	switch c.FinishReason {
	case "length":
		return nil, errors.New("the response was cut off at the token limit; the document is too large for this model or its context is too small")
	case "content_filter":
		return nil, errors.New("the response was blocked by the provider's content filter")
	}
	return ExtractJSON(c.Message.Content)
}

// apiMessage extracts the error text from an API error body, falling back to
// the (truncated) raw body. Gateways such as OpenRouter wrap the real reason in
// error.metadata ("Provider returned error" plus the upstream provider's own
// message), so that detail is appended when present.
func apiMessage(data []byte) string {
	var e struct {
		Error struct {
			Message  string `json:"message"`
			Metadata struct {
				Raw          json.RawMessage `json:"raw"`
				ProviderName string          `json:"provider_name"`
			} `json:"metadata"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
		msg := e.Error.Message
		var detail string
		if raw := e.Error.Metadata.Raw; len(raw) > 0 {
			var str string
			if json.Unmarshal(raw, &str) == nil {
				detail = str
			} else {
				detail = string(raw)
			}
		}
		if detail = strings.TrimSpace(detail); len(detail) > 300 {
			detail = detail[:300] + "..."
		}
		switch provider := e.Error.Metadata.ProviderName; {
		case provider != "" && detail != "":
			msg += " (" + provider + ": " + detail + ")"
		case provider != "":
			msg += " (" + provider + ")"
		case detail != "":
			msg += " (" + detail + ")"
		}
		return msg
	}
	s := strings.TrimSpace(string(data))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
