package tailor

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/BernatRubio/best-candidate/internal/llm"
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
func Run(ctx context.Context, p llm.Provider, profile, offer string, opt Options) (*Result, error) {
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

	g := newGrounder(profile)
	catalog := courseCatalog(profile)
	opt.logf("tailoring the documents")
	docs, err := tailorDocs(ctx, p, profile, offer, analysis, lang, nil, "")
	if err != nil {
		return nil, err
	}

	res := &Result{Analysis: analysis, Lang: lang}
	var verifierCoverage []Coverage
	for round := 0; ; round++ {
		opt.logf("verifying every claim against the profile (round %d)", round+1)
		issues, coverage, err := check(ctx, p, g, profile, offer, analysis, docs)
		if err != nil {
			return nil, err
		}
		verifierCoverage = coverage
		// A course named under two degrees is not a false claim, so it is
		// repaired (or, as a last resort, deduplicated) rather than dropped.
		dups := duplicateCourseIssues(docs, catalog)
		if len(issues) == 0 && len(dups) == 0 {
			break
		}
		opt.logf("%d claim(s) flagged", len(issues)+len(dups))
		if round >= opt.MaxRepairs {
			res.Dropped = drop(docs, issues)
			res.Warnings = append(res.Warnings, dedupeCoursework(docs, catalog)...)
			break
		}
		opt.logf("sending the flagged claims back for correction")
		prev, _ := json.MarshalIndent(docs, "", "  ")
		docs, err = tailorDocs(ctx, p, profile, offer, analysis, lang, prev, problemList(docs, mergeIssues(issues, dups)))
		if err != nil {
			return nil, err
		}
	}

	res.Warnings = append(res.Warnings, groundIdentity(g, docs)...)
	res.Warnings = append(res.Warnings, groundRankings(g, docs)...)
	res.Warnings = append(res.Warnings, groundProjectLinks(g, docs)...)
	cleanEducationTitles(docs)
	cleanRecipient(docs, analysis)
	res.Warnings = append(res.Warnings, fillSkillsLanguages(profile, docs)...)
	// The fact-checker's assessment of the requirements is preferred: it has no
	// stake in the candidate looking good, and some writers skip it entirely.
	if len(verifierCoverage) > 0 {
		docs.Coverage = verifierCoverage
	}
	groundCoverage(g, docs)
	res.Docs = *docs
	return res, nil
}

func analyze(ctx context.Context, p llm.Provider, offer string) (JobAnalysis, error) {
	raw, err := p.GenerateJSON(ctx, llm.Request{
		System: analyzePrompt,
		User:   tag("job_offer", offer),
		Schema: llm.SchemaFor[JobAnalysis](),
	})
	if err != nil {
		return JobAnalysis{}, fmt.Errorf("analysing job offer: %w", err)
	}
	return decode[JobAnalysis](raw, "job analysis")
}

func tailorDocs(ctx context.Context, p llm.Provider, profile, offer string, a JobAnalysis, lang string, prev []byte, problems string) (*Documents, error) {
	analysisJSON, _ := json.MarshalIndent(a, "", "  ")
	user := tag("profile", profile) +
		tag("job_offer", offer) +
		tag("job_analysis", string(analysisJSON)) +
		tag("language", lang)
	if prev != nil {
		user += tag("previous_output", string(prev)) + tag("problems", problems)
	}
	raw, err := p.GenerateJSON(ctx, llm.Request{
		System: tailorPrompt,
		User:   user,
		Schema: llm.SchemaFor[Documents](),
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

// check returns id -> reason for every claim that is not fully supported. It
// combines mechanical quote checks with an independent model review.
func check(ctx context.Context, p llm.Provider, g *grounder, profile, offer string, analysis JobAnalysis, docs *Documents) (map[string]string, []Coverage, error) {
	claims := docs.claims()
	issues := map[string]string{}

	for _, c := range claims {
		if c.NeedSource && len(c.Sources) == 0 {
			issues[c.ID] = "no source quote given"
			continue
		}
		if miss := g.missing(c.Sources); len(miss) > 0 {
			issues[c.ID] = fmt.Sprintf("quoted source not found in the profile: %q", miss[0])
			continue
		}
		if isEducationDetail(c.ID) && refersToOtherEntry(c.Text) {
			issues[c.ID] = "refers to another entry or to the other degree; every degree entry must stand on its own, so state the fact fully inside this entry or delete the line"
		} else if isEducationDetail(c.ID) && courseworkLeadHasRemark(c.Text) {
			issues[c.ID] = "the coursework line must hold only the list of courses; move the remark in parentheses (for example the language of instruction) to its own line"
		}
	}

	var list strings.Builder
	for _, c := range claims {
		fmt.Fprintf(&list, "id: %s\ntext: %s\ncited: %s\n\n", c.ID, c.Text, strings.Join(c.Sources, " / "))
	}
	raw, err := p.GenerateJSON(ctx, llm.Request{
		System: verifyPrompt,
		User:   tag("profile", profile) + tag("job_offer", offer) + requirementsTag(analysis) + tag("claims", list.String()),
		Schema: llm.SchemaFor[VerificationReport](),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("verifying documents: %w", err)
	}
	report, err := decode[VerificationReport](raw, "verification report")
	if err != nil {
		return nil, nil, err
	}

	seen := map[string]bool{}
	for _, v := range report.Verdicts {
		seen[v.ID] = true
		if v.Status != "supported" {
			if prev, ok := issues[v.ID]; ok {
				issues[v.ID] = prev + "; " + v.Status + ": " + v.Reason
			} else {
				issues[v.ID] = v.Status + ": " + v.Reason
			}
		}
	}
	for _, c := range claims {
		if !seen[c.ID] {
			if _, ok := issues[c.ID]; !ok {
				issues[c.ID] = "not reviewed by the fact-checker"
			}
		}
	}
	return issues, report.Coverage, nil
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

func problemList(docs *Documents, issues map[string]string) string {
	var b strings.Builder
	for _, c := range docs.claims() {
		if reason, ok := issues[c.ID]; ok {
			fmt.Fprintf(&b, "- %s: %q -> %s\n", c.ID, c.Text, reason)
		}
	}
	return b.String()
}

func drop(docs *Documents, issues map[string]string) []Dropped {
	var out []Dropped
	bad := map[string]bool{}
	for _, c := range docs.claims() {
		if reason, ok := issues[c.ID]; ok {
			bad[c.ID] = true
			out = append(out, Dropped{ID: c.ID, Text: c.Text, Reason: reason})
		}
	}
	docs.prune(bad)
	return out
}

// groundIdentity makes sure the name and contact lines really come from the
// profile; unverifiable contact lines are removed.
func groundIdentity(g *grounder, docs *Documents) []string {
	var warnings []string
	if !g.has(docs.CV.Name) {
		warnings = append(warnings, fmt.Sprintf("name %q was not found in the profile", docs.CV.Name))
	}
	var contact []string
	for _, line := range docs.CV.Contact {
		if g.has(line) {
			contact = append(contact, line)
		} else {
			warnings = append(warnings, fmt.Sprintf("contact line %q is not in the profile and was removed", line))
		}
	}
	docs.CV.Contact = contact
	return warnings
}

// groundCoverage downgrades "covered" entries whose evidence quote is not in
// the profile.
func groundCoverage(g *grounder, docs *Documents) {
	for i, c := range docs.Coverage {
		if c.Covered && !g.has(c.Evidence) {
			docs.Coverage[i].Covered = false
			docs.Coverage[i].Evidence = ""
		}
	}
}

var (
	educationDetailID = regexp.MustCompile(`^cv\.education\[\d+\]\.details\[\d+\]$`)
	crossReference    = regexp.MustCompile(`(?i)\b(same as above|as above|as (?:in|for) the (?:previous|other|first|second)|the (?:previous|other|first|second) degree|both degrees|both of (?:my|the) degrees|likewise|similarly|also)\b`)
)

func isEducationDetail(id string) bool { return educationDetailID.MatchString(id) }

// refersToOtherEntry reports whether an education line points at another entry
// ("also taught in English", "same as above"), which makes no sense when the
// entries are read on their own.
func refersToOtherEntry(text string) bool { return crossReference.MatchString(text) }

// groundRankings clears a class ranking that is not in the profile (a wrong
// ranking is worse than none) and reports it.
func groundRankings(g *grounder, docs *Documents) []string {
	var warnings []string
	for i, e := range docs.CV.Education {
		if e.Ranking != "" && !g.has(e.Ranking) {
			warnings = append(warnings, fmt.Sprintf("class ranking %q for %q is not in the profile and was removed", e.Ranking, e.Degree))
			docs.CV.Education[i].Ranking = ""
		}
	}
	return warnings
}

// mergeIssues combines two id -> reason maps; reasons for the same id are joined.
func mergeIssues(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for id, r := range a {
		out[id] = r
	}
	for id, r := range b {
		if prev, ok := out[id]; ok {
			out[id] = prev + "; " + r
		} else {
			out[id] = r
		}
	}
	return out
}

var (
	trailingParen = regexp.MustCompile(`\s*\(([^)]*)\)\s*$`)
	yearRange     = regexp.MustCompile(`^\d{4}\s*[-–—]\s*(\d{4}|[A-Za-z]+)$`)
)

// cleanEducationTitles removes what weaker models paste into the degree name
// from the profile heading: the dates ("(2020 - 2025)") and the institution
// (", Oxford"), which the entry already shows in their own places.
func cleanEducationTitles(docs *Documents) {
	for i := range docs.CV.Education {
		e := &docs.CV.Education[i]
		d := strings.TrimSpace(e.Degree)
		if m := trailingParen.FindStringSubmatch(d); m != nil {
			inner := strings.TrimSpace(m[1])
			if yearRange.MatchString(inner) || (e.Period != "" && normalize(inner) == normalize(e.Period)) {
				d = strings.TrimSpace(trailingParen.ReplaceAllString(d, ""))
			}
		}
		if k := strings.LastIndex(d, ", "); k > 0 {
			tail := strings.TrimSpace(d[k+2:])
			inst := " " + normalize(e.Institution) + " "
			if t := normalize(tail); t != "" && len(strings.Fields(t)) <= 6 && strings.Contains(inst, " "+t+" ") {
				d = strings.TrimSpace(d[:k])
			}
		}
		if d != "" {
			e.Degree = d
		}
	}
}

var identifierLike = regexp.MustCompile(`^[A-Za-z0-9]+(_[A-Za-z0-9]+)+$`)

// cleanRecipient replaces an addressee that is a placeholder identifier
// ("hiring_team") with the company name, or with nothing when the company is
// unknown. The model returns such values when it treats the field as a label.
func cleanRecipient(docs *Documents, a JobAnalysis) {
	r := strings.TrimSpace(docs.Letter.Recipient)
	if identifierLike.MatchString(r) || strings.Contains(r, "_") {
		docs.Letter.Recipient = strings.TrimSpace(a.Company)
	}
}

var courseworkRemark = regexp.MustCompile(`(?i)^\s*[^:()]*\b(coursework|courses)\s*\([^)]*\)\s*:`)

// courseworkLeadHasRemark reports whether a coursework line carries a remark in
// parentheses before its colon, as in "Relevant coursework (degree studied
// entirely in English): ...".
func courseworkLeadHasRemark(text string) bool { return courseworkRemark.MatchString(text) }
