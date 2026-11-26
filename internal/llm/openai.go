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

// submitTool is the tool the model must call: its parameters are the schema,
// which works on servers that ignore response_format (such as OpenRouter's
// free endpoints).
const submitTool = "submit"

// OpenAICompatConfig configures a client for the /chat/completions API shared
// by OpenAI, OpenRouter, Ollama and others.
type OpenAICompatConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// OpenAICompat talks to any OpenAI-compatible chat completions endpoint.
type OpenAICompat struct {
	cfg  OpenAICompatConfig
	http *http.Client
}

// NewOpenAICompat builds the client.
func NewOpenAICompat(cfg OpenAICompatConfig) *OpenAICompat {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &OpenAICompat{
		cfg:  cfg,
		http: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (o *OpenAICompat) Name() string { return o.cfg.Model }

// compatRetry retries rate limits and overload: the waits are 2s, 4s, 8s and
// 16s, or the server's Retry-After (up to 30s).
var compatRetry = RetryPolicy{Attempts: 5, Base: 2 * time.Second, Transient: isTransientCompat}

// isTransientCompat reports whether a call failed in a way worth retrying.
func isTransientCompat(err error) bool {
	var re *retryableError
	return errors.As(err, &re)
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			Refusal   string `json:"refusal"`
			ToolCalls []struct {
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// GenerateJSON requests a JSON document matching req.Schema. Rate limits and
// transient server errors fail with a retryableError (see WithRetry).
func (o *OpenAICompat) GenerateJSON(ctx context.Context, req Request) (json.RawMessage, error) {
	payload := map[string]any{
		"model": o.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": req.System},
			{"role": "user", "content": req.User},
		},
		"tools": []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name":        submitTool,
				"description": "Submit the final answer. Its arguments are the whole JSON document.",
				"parameters":  req.Schema,
			},
		}},
		"tool_choice": map[string]any{"type": "function", "function": map[string]string{"name": submitTool}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return o.once(ctx, body)
}

type retryableError struct {
	err   error
	after time.Duration
}

func (e *retryableError) Error() string             { return e.err.Error() }
func (e *retryableError) Unwrap() error             { return e.err }
func (e *retryableError) RetryAfter() time.Duration { return e.after }

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
		return nil, &retryableError{err: fmt.Errorf("request: %w", err)}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, &retryableError{err: fmt.Errorf("reading response: %w", err)}
	}

	if resp.StatusCode != http.StatusOK {
		apiErr := fmt.Errorf("API error %d: %s", resp.StatusCode, apiMessage(data))
		if resp.StatusCode == http.StatusTooManyRequests && strings.Contains(string(data), "per-day") {
			// A daily quota (such as OpenRouter's free-models-per-day) does not
			// clear within any retry wait.
			return nil, fmt.Errorf("%w (the daily request quota is used up: try again tomorrow or use another model)", apiErr)
		}
		switch resp.StatusCode {
		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			re := &retryableError{err: apiErr}
			if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 && secs <= 30 {
				re.after = time.Duration(secs) * time.Second
			}
			return nil, re
		case http.StatusBadRequest:
			return nil, fmt.Errorf("%w (check that the model supports tool calls)", apiErr)
		case http.StatusUnauthorized, http.StatusForbidden:
			if o.cfg.APIKey == "" {
				return nil, fmt.Errorf("%w (no API key was sent: set BC_API_KEY in the shell or in .env)", apiErr)
			}
			return nil, fmt.Errorf("%w (the API key was rejected: check it and that it belongs to this provider)", apiErr)
		}
		return nil, apiErr
	}

	var cr chatResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if cr.Error != nil {
		return nil, fmt.Errorf("API error: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return nil, errors.New("the API returned no choices")
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
	for _, tc := range c.Message.ToolCalls {
		if tc.Function.Name == submitTool || len(c.Message.ToolCalls) == 1 {
			return ExtractJSON(tc.Function.Arguments)
		}
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
