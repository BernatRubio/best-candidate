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

//go:embed prompts/analyze.md
var analyzePrompt string

//go:embed prompts/tailor.md
var tailorPrompt string

//go:embed prompts/verify.md
var verifyPrompt string

// Options tune a pipeline run.
type Options struct {
	// Lang forces the output language (ISO 639-1). Empty or "auto" follows the offer.
	Lang string
	// MaxRepairs is how many times flagged claims are sent back for correction
	// before the remaining ones are dropped.
	MaxRepairs int
	// Logf receives progress messages; nil silences them.
	Logf func(format string, args ...any)
}

// Dropped is a claim removed because it could not be grounded in the profile.
type Dropped struct {
	ID     string
	Text   string
	Reason string
}

// Result is the outcome of a run.
type Result struct {
	Analysis JobAnalysis
	Lang     string
	Docs     Documents
	Dropped  []Dropped
	Warnings []string
	ATS      ATS

	ats atsInput
}

// atsInput is what the ATS assessment needs besides the CV.
type atsInput struct {
	prof     *profile.Profile
	analysis JobAnalysis
	support  []KeywordSupport
}

// RefreshATS recomputes ATS from the current CV, for callers that change the
// documents after Run (such as cutting content to fit one page).
func (r *Result) RefreshATS() {
	if r.ats.prof == nil {
		return
	}
	r.ATS, _ = keywordStatus(r.ats.prof, &r.Docs, r.ats.analysis, r.ats.support)
}

func (o Options) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

func tag(name, body string) string {
	return "<" + name + ">\n" + strings.TrimSpace(body) + "\n</" + name + ">\n\n"
}

func decode[T any](raw json.RawMessage, what string) (T, error) {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("decoding %s: %w", what, err)
	}
	return v, nil
}

// Run produces CV and letter documents for the offer, with every claim checked
// against the profile. Claims that cannot be grounded after the repair rounds
// are dropped and listed in Result.Dropped.
func Run(ctx context.Context, p llm.Provider, prof *profile.Profile, offer string, opt Options) (*Result, error) {
	opt.logf("analysing the job offer")
	analysis, err := analyze(ctx, p, offer)
	if err != nil {
		return nil, err
	}

	lang := strings.ToLower(strings.TrimSpace(opt.Lang))
	if lang == "" || lang == "auto" {
		lang = strings.ToLower(analysis.Language)
	}
	if lang == "" {
		lang = "en"
	}
	opt.logf("offer: %s at %s (writing in %q)", analysis.Role, analysis.Company, lang)

	rv := newDraftReview(prof, analysis)
	opt.logf("tailoring the documents")
	docs, err := tailorDocs(ctx, p, prof.Text(), offer, analysis, lang, nil, "", "")
	if err != nil {
		return nil, err
	}
	rv.Prepare(docs)

	res := &Result{Analysis: analysis, Lang: lang}
	var report VerificationReport
	for round := 0; ; round++ {
		opt.logf("verifying every claim against the profile (round %d)", round+1)
		if report, err = verify(ctx, p, prof.Text(), offer, analysis, docs); err != nil {
			return nil, err
		}
		f := rv.Review(docs, report)
		if f.clean() {
			break
		}
		opt.logf("%d claim(s) flagged, %d keyword(s) missing", len(f.Issues)+len(f.Courses), len(f.Gaps))
		if round >= opt.MaxRepairs {
			var warnings []string
			res.Dropped, warnings = rv.GiveUp(docs, f)
			res.Warnings = append(res.Warnings, warnings...)
			break
		}
		opt.logf("sending the flagged claims back for correction")
		prev, _ := json.MarshalIndent(docs, "", "  ")
		if docs, err = tailorDocs(ctx, p, prof.Text(), offer, analysis, lang, prev, f.problems(docs), f.missingKeywords(analysis)); err != nil {
			return nil, err
		}
		rv.Prepare(docs)
	}

	res.Warnings = append(res.Warnings, rv.Finalize(docs, report)...)
	res.Docs = *docs
	// Computed last, so that every change to the CV above is counted.
	res.ats = atsInput{prof: prof, analysis: analysis, support: report.Keywords}
	res.RefreshATS()
	return res, nil
}

func analyze(ctx context.Context, p llm.Provider, offer string) (JobAnalysis, error) {
	raw, err := p.GenerateJSON(ctx, llm.Request{
		System: analyzePrompt,
		User:   tag("job_offer", offer) + "Extract the structure of this job offer.",
		Schema: llm.SchemaFor[JobAnalysis](),
	})
	if err != nil {
		return JobAnalysis{}, fmt.Errorf("analysing job offer: %w", err)
	}
	return decode[JobAnalysis](raw, "job analysis")
}

// draft is what the writer returns: Documents without the coverage, which the
// fact-checker assesses.
type draft struct {
	CV     CV     `json:"cv"`
	Letter Letter `json:"letter"`
}

func tailorDocs(ctx context.Context, p llm.Provider, profile, offer string, a JobAnalysis, lang string, prev []byte, problems, missing string) (*Documents, error) {
	analysisJSON, _ := json.MarshalIndent(a, "", "  ")
	user := tag("profile", profile) +
		tag("job_offer", offer) +
		tag("job_analysis", string(analysisJSON)) +
		tag("language", lang)
	task := "Write the tailored CV and cover letter for this offer, following every rule, then return the JSON document."
	if prev != nil {
		user += tag("previous_output", string(prev)) + tag("problems", problems)
		if missing != "" {
			user += tag("missing_keywords", missing)
		}
		task = "Repair <previous_output> as described under Repairs and return the complete corrected JSON document."
	}
	user += task
	raw, err := p.GenerateJSON(ctx, llm.Request{
		System: tailorPrompt,
		User:   user,
		Schema: llm.SchemaFor[draft](),
	})
	if err != nil {
		return nil, fmt.Errorf("tailoring documents: %w", err)
	}
	docs, err := decode[Documents](raw, "documents")
	if err != nil {
		return nil, err
	}
	return &docs, nil
}

// verify asks the fact-checker for a verdict on every claim, the coverage of
// the requirements and the support for each keyword. Some models stop part way
// through a long list, so the claims they skip are asked about once more, on
// their own.
func verify(ctx context.Context, p llm.Provider, profile, offer string, analysis JobAnalysis, docs *Documents) (VerificationReport, error) {
	claims := docs.claims()
	report, err := review(ctx, p, profile, offer, requirementsTag(analysis)+keywordsTag(analysis), claims)
	if err != nil {
		return report, err
	}
	seen := map[string]bool{}
	for _, v := range report.Verdicts {
		seen[v.ID] = true
	}
	var skipped []flatClaim
	for _, c := range claims {
		if !seen[c.ID] {
			skipped = append(skipped, c)
		}
	}
	if len(skipped) > 0 {
		more, err := review(ctx, p, profile, offer, "", skipped)
		if err != nil {
			return report, err
		}
		report.Verdicts = append(report.Verdicts, more.Verdicts...)
	}
	return report, nil
}

// review asks the fact-checker for a verdict on each claim. extra holds the
// optional requirements and keywords blocks.
func review(ctx context.Context, p llm.Provider, profile, offer, extra string, claims []flatClaim) (VerificationReport, error) {
	var list strings.Builder
	for _, c := range claims {
		fmt.Fprintf(&list, "id: %s\ntext: %s\ncited: %s\n\n", c.ID, c.Text, strings.Join(c.Sources, " / "))
	}
	raw, err := p.GenerateJSON(ctx, llm.Request{
		System: verifyPrompt,
		User: tag("profile", profile) + tag("job_offer", offer) + extra + tag("claims", list.String()) +
			fmt.Sprintf("Fact-check all %d claims above against the profile and return one verdict per id.", len(claims)),
		Schema: llm.SchemaFor[VerificationReport](),
	})
	if err != nil {
		return VerificationReport{}, fmt.Errorf("verifying documents: %w", err)
	}
	return decode[VerificationReport](raw, "verification report")
}

// requirementsTag lists the offer's requirements for the fact-checker to assess.
func requirementsTag(a JobAnalysis) string {
	var b strings.Builder
	for _, r := range a.MustHave {
		fmt.Fprintf(&b, "must have: %s\n", r)
	}
	for _, r := range a.NiceToHave {
		fmt.Fprintf(&b, "nice to have: %s\n", r)
	}
	if b.Len() == 0 {
		return ""
	}
	return tag("requirements", b.String())
}
