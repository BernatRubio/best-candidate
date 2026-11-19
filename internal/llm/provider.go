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

// Config selects and configures a provider.
type Config struct {
	Provider string // anthropic | openai | gemini | ollama | openai-compatible
	Model    string
	BaseURL  string // openai-compatible and ollama; optional elsewhere
}

// Supported lists the provider names New accepts.
const Supported = "anthropic, openai, gemini, ollama, openai-compatible"

// New builds a provider. Only Anthropic has a default model: model ids of the
// other vendors change too often to hard-code, so they must be given.
func New(c Config) (Provider, error) {
	name := strings.ToLower(strings.TrimSpace(c.Provider))
	if name == "" {
		name = "anthropic"
	}
	if name != "anthropic" && name != "claude" && c.Model == "" {
		return nil, fmt.Errorf("provider %q needs a model: pass --model or set BC_MODEL", name)
	}
	mode := firstNonEmpty(os.Getenv("BC_JSON_MODE"), JSONModeSchema)

	switch name {
	case "anthropic", "claude":
		return NewAnthropic(c.Model)
	case "openai":
		return NewOpenAICompat(OpenAICompatConfig{
			Name:        "openai",
			BaseURL:     firstNonEmpty(c.BaseURL, "https://api.openai.com/v1"),
			APIKey:      firstNonEmpty(os.Getenv("BC_API_KEY"), os.Getenv("OPENAI_API_KEY")),
			KeyRequired: "OPENAI_API_KEY",
			Model:       c.Model,
			JSONMode:    mode,
		})
	case "ollama":
		return NewOpenAICompat(OpenAICompatConfig{
			Name:     "ollama",
			BaseURL:  firstNonEmpty(c.BaseURL, "http://localhost:11434/v1"),
			APIKey:   os.Getenv("BC_API_KEY"),
			Model:    c.Model,
			JSONMode: mode,
		})
	case "openai-compatible", "compatible", "compat":
		if c.BaseURL == "" {
			return nil, fmt.Errorf("provider %q needs a base URL: pass --base-url or set BC_BASE_URL", name)
		}
		return NewOpenAICompat(OpenAICompatConfig{
			Name:     "compat",
			BaseURL:  c.BaseURL,
			APIKey:   firstNonEmpty(os.Getenv("BC_API_KEY"), os.Getenv("OPENAI_API_KEY")),
			Model:    c.Model,
			JSONMode: mode,
		})
	case "gemini", "google":
		return NewGemini(c.Model)
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: %s)", c.Provider, Supported)
	}
}

// FromEnv builds a provider from BC_PROVIDER / BC_MODEL / BC_BASE_URL, using
// the flag values as the fallback.
func FromEnv(flags Config) (Provider, error) {
	return New(Config{
		Provider: firstNonEmpty(os.Getenv("BC_PROVIDER"), flags.Provider),
		Model:    firstNonEmpty(os.Getenv("BC_MODEL"), flags.Model),
		BaseURL:  firstNonEmpty(os.Getenv("BC_BASE_URL"), flags.BaseURL),
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
// markdown code fence or surrounded by prose, as weaker models tend to do.
func ExtractJSON(text string) (json.RawMessage, error) {
	s := strings.TrimSpace(text)
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
