package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"google.golang.org/genai"
)

// geminiRetry waits 3s, 6s, 12s and 24s.
var geminiRetry = RetryPolicy{Attempts: 5, Base: 3 * time.Second, Transient: isTransientGemini}

const (
	// geminiTimeout bounds one attempt: the SDK client has no timeout of its own, so a
	// stalled connection would otherwise wait forever.
	geminiTimeout = 4 * time.Minute
)

// Gemini calls Google's Gemini API with schema-constrained JSON output.
type Gemini struct {
	client  *genai.Client
	model   string
	timeout time.Duration
}

// NewGemini builds the Gemini provider. The key comes from GEMINI_API_KEY or
// GOOGLE_API_KEY.
func NewGemini(model string) (*Gemini, error) {
	key := firstNonEmpty(os.Getenv("GEMINI_API_KEY"), os.Getenv("GOOGLE_API_KEY"))
	if key == "" {
		return nil, errors.New("GEMINI_API_KEY (or GOOGLE_API_KEY) is not set")
	}
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  key,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("creating Gemini client: %w", err)
	}
	return &Gemini{client: client, model: model, timeout: geminiTimeout}, nil
}

func (g *Gemini) Name() string { return "gemini/" + g.model }

// isTransientGemini reports whether an error is worth retrying: rate limits
// (429) and temporary server trouble such as "model is overloaded" (500, 502,
// 503, 504).
func isTransientGemini(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true // an attempt timed out (the caller's own context is checked by retry)
	}
	var apiErr genai.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Code {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// GenerateJSON asks Gemini for a JSON document matching req.Schema. One call is
// bounded by the timeout; retries are added by WithRetry.
func (g *Gemini) GenerateJSON(ctx context.Context, req Request) (json.RawMessage, error) {
	var schema map[string]any
	if err := json.Unmarshal(req.Schema, &schema); err != nil {
		return nil, fmt.Errorf("reading schema: %w", err)
	}
	cfg := &genai.GenerateContentConfig{
		SystemInstruction:  genai.NewContentFromText(req.System, genai.RoleUser),
		ResponseMIMEType:   "application/json",
		ResponseJsonSchema: schema,
		MaxOutputTokens:    16384,
	}
	contents := []*genai.Content{genai.NewContentFromText(req.User, genai.RoleUser)}

	attempt, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	resp, err := g.client.Models.GenerateContent(attempt, g.model, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini request: %w", err)
	}

	if len(resp.Candidates) == 0 {
		if pf := resp.PromptFeedback; pf != nil && pf.BlockReason != "" {
			return nil, fmt.Errorf("the request was blocked by Gemini (%s)", pf.BlockReason)
		}
		return nil, errors.New("gemini returned no candidates")
	}
	switch fr := resp.Candidates[0].FinishReason; fr {
	case "", genai.FinishReasonStop, genai.FinishReasonUnspecified:
	case genai.FinishReasonMaxTokens:
		return nil, errors.New("the response was cut off at the token limit; the document is too large")
	default:
		return nil, fmt.Errorf("gemini stopped without an answer (finish reason %s)", fr)
	}
	return ExtractJSON(resp.Text())
}
