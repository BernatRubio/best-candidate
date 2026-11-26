// Package llm hides the model provider behind a single structured-output call.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/invopop/jsonschema"
)

// Request is one structured-output call: the model must answer with JSON that
// validates against Schema.
type Request struct {
	System string
	User   string
	Schema json.RawMessage
}

// Provider generates JSON documents from a model.
type Provider interface {
	Name() string
	GenerateJSON(ctx context.Context, req Request) (json.RawMessage, error)
}

// Config configures the OpenAI-compatible provider.
type Config struct {
	Model   string
	BaseURL string
}

// DefaultBaseURL is OpenRouter's API.
const DefaultBaseURL = "https://openrouter.ai/api/v1"

// New builds an OpenAI-compatible provider with retries. The API key is read
// from BC_API_KEY.
func New(c Config) (Provider, error) {
	if c.Model == "" {
		return nil, fmt.Errorf("a model is needed: pass --model or set BC_MODEL")
	}
	o := NewOpenAICompat(OpenAICompatConfig{
		BaseURL: firstNonEmpty(c.BaseURL, DefaultBaseURL),
		APIKey:  os.Getenv("BC_API_KEY"),
		Model:   c.Model,
	})
	return WithRetry(o, compatRetry), nil
}

// FromEnv builds a provider from BC_MODEL / BC_BASE_URL, using the flag values
// as the fallback.
func FromEnv(flags Config) (Provider, error) {
	return New(Config{
		Model:   firstNonEmpty(os.Getenv("BC_MODEL"), flags.Model),
		BaseURL: firstNonEmpty(os.Getenv("BC_BASE_URL"), flags.BaseURL),
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ExtractJSON pulls one JSON value out of model text that may be wrapped in a
// markdown code fence or surrounded by prose, as weaker models tend to do. A
// reasoning block (<think>...</think>) that leaked into the text is skipped.
func ExtractJSON(text string) (json.RawMessage, error) {
	s := strings.TrimSpace(text)
	if i := strings.LastIndex(s, "</think>"); i >= 0 {
		s = strings.TrimSpace(s[i+len("</think>"):])
	}
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimPrefix(s, "json")
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}
	if json.Valid([]byte(s)) {
		return json.RawMessage(s), nil
	}
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i && json.Valid([]byte(s[i:j+1])) {
		return json.RawMessage(s[i : j+1]), nil
	}
	return nil, fmt.Errorf("model did not return valid JSON: %.200s", text)
}

// SchemaFor derives a strict JSON schema (no extra properties, no $ref) from a
// Go type, in the shape structured-output APIs accept.
func SchemaFor[T any]() json.RawMessage {
	r := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	s := r.Reflect(new(T))
	raw, err := json.Marshal(s)
	if err != nil {
		panic(fmt.Sprintf("marshalling schema: %v", err))
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(fmt.Sprintf("re-reading schema: %v", err))
	}
	delete(m, "$schema")
	delete(m, "$id")
	out, err := json.Marshal(m)
	if err != nil {
		panic(fmt.Sprintf("marshalling schema: %v", err))
	}
	return out
}
