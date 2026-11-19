package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

// DefaultAnthropicModel is used when no model is configured.
const DefaultAnthropicModel = "claude-opus-5-5"

// Anthropic calls Claude. Credentials come from ANTHROPIC_API_KEY or an
// `ant auth login` profile, resolved by the SDK.
type Anthropic struct {
	client anthropic.Client
	model  string
}

// NewAnthropic builds the Claude provider.
func NewAnthropic(model string) (*Anthropic, error) {
	if model == "" {
		model = DefaultAnthropicModel
	}
	return &Anthropic{client: anthropic.NewClient(), model: model}, nil
}

func (a *Anthropic) Name() string { return "anthropic/" + a.model }

// GenerateJSON asks Claude for a JSON document matching req.Schema. It streams
// the response (high output + thinking can exceed plain request timeouts) and
// enables server-side refusal fallback.
func (a *Anthropic) GenerateJSON(ctx context.Context, req Request) (json.RawMessage, error) {
	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: 32000,
		System:    []anthropic.BetaTextBlockParam{{Text: req.System}},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(req.User)),
		},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortHigh,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: req.Schema},
		},
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
	}

	stream := a.client.Beta.Messages.NewStreaming(ctx, params)
	msg := anthropic.BetaMessage{}
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return nil, fmt.Errorf("accumulating response: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return nil, fmt.Errorf("anthropic API error %d: %w", apiErr.StatusCode, err)
		}
		return nil, fmt.Errorf("anthropic request: %w", err)
	}

	switch msg.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return nil, errors.New("the model declined this request (stop_reason=refusal)")
	case anthropic.BetaStopReasonMaxTokens:
		return nil, errors.New("the response was cut off at max_tokens; the document is too large")
	}

	var text strings.Builder
	for _, block := range msg.Content {
		if tb, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(tb.Text)
		}
	}
	out := strings.TrimSpace(text.String())
	if !json.Valid([]byte(out)) {
		return nil, fmt.Errorf("model returned invalid JSON: %.200s", out)
	}
	return json.RawMessage(out), nil
}
