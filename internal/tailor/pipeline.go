package tailor

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BernatRubio/best-candidate/internal/llm"
	"github.com/BernatRubio/best-candidate/internal/profile"
)

//go:embed prompts/tailor.md
var tailorPrompt string

// Options tune a run.
type Options struct {
	// Lang forces the output language (ISO 639-1). Empty or "auto" follows the offer.
	Lang string
	// Logf receives progress messages; nil silences them.
	Logf func(format string, args ...any)
}

// Result is the outcome of a run.
type Result struct {
	Company  string
	Role     string
	Lang     string
	Docs     Documents
	Keywords []KeywordHit
	Warnings []string
}

// Run asks the model, in one call, for a CV and cover letter tailored to the
// offer.
func Run(ctx context.Context, p llm.Provider, prof *profile.Profile, offer string, opt Options) (*Result, error) {
	lang := strings.ToLower(strings.TrimSpace(opt.Lang))
	if lang == "auto" {
		lang = ""
	}
	langTag := lang
	if langTag == "" {
		langTag = "the language the job offer is written in"
	}

	if opt.Logf != nil {
		opt.Logf("tailoring the documents")
	}
	raw, err := p.GenerateJSON(ctx, llm.Request{
		System: tailorPrompt,
		User: tag("profile", prof.Text()) +
			tag("job_offer", offer) +
			tag("language", langTag) +
			"Write the tailored CV and cover letter for this offer, following every rule, then return the JSON document.",
		Schema: llm.SchemaFor[output](),
	})
	if err != nil {
		return nil, fmt.Errorf("tailoring documents: %w", err)
	}
	var out output
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding documents: %w", err)
	}

	if lang == "" {
		lang = strings.ToLower(strings.TrimSpace(out.Language))
	}
	if lang == "" {
		lang = "en"
	}
	hits := Coverage(out.Keywords, out.CV)
	var warnings []string
	for _, h := range hits {
		if h.Supported && h.Count == 0 {
			warnings = append(warnings, MissingWarning(h))
		}
	}
	return &Result{
		Company:  out.Company,
		Role:     out.Role,
		Lang:     lang,
		Docs:     Documents{CV: out.CV, Letter: out.Letter},
		Keywords: hits,
		Warnings: warnings,
	}, nil
}

func tag(name, body string) string {
	return "<" + name + ">\n" + strings.TrimSpace(body) + "\n</" + name + ">\n\n"
}
