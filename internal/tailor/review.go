package tailor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/BernatRubio/best-candidate/internal/profile"
)

// draftReview holds everything the program checks and fixes in a draft by
// itself, around the model calls. The order of its passes matters (education
// is sorted before any claim ref is taken, skills are filled before the ATS
// count) and is decided here only.
type draftReview struct {
	prof     *profile.Profile
	analysis JobAnalysis
	catalog  []string // the profile's courses
}

func newDraftReview(prof *profile.Profile, a JobAnalysis) *draftReview {
	return &draftReview{prof: prof, analysis: a, catalog: prof.Courses()}
}

// findings is what one review round found wrong with a draft.
type findings struct {
	// Issues are claims the profile does not support (or that break a rule the
	// program checks); they are dropped if they persist.
	Issues map[string]string
	// Courses are courses named under two degrees; not false claims, so they
	// are deduplicated rather than dropped if they persist.
	Courses map[string]string
	// Gaps are keywords the profile supports but the CV under-uses; they only
	// trigger a repair round.
	Gaps map[string]keywordGap
	// Unreviewed are claims the fact-checker gave no verdict for, in document
	// order. The writer cannot fix them, so they are not sent for repair: they
	// are verified again and dropped if they never get a verdict.
	Unreviewed []string

	support []KeywordSupport // the fact-checker's keyword verdicts
}

func (f findings) clean() bool {
	return len(f.Issues)+len(f.Courses)+len(f.Gaps)+len(f.Unreviewed) == 0
}

// onlyUnreviewed reports whether the fact-checker's silence is all that is
// wrong, so that verifying again is enough and the writer need not run.
func (f findings) onlyUnreviewed() bool {
	return len(f.Unreviewed) > 0 && len(f.Issues)+len(f.Courses)+len(f.Gaps) == 0
}

const notReviewed = "not reviewed by the fact-checker"

// Prepare runs right after every tailoring call, before any claim ref is
// taken: the degrees are put in order of relevance, so that a course shared by
// two degrees is kept under the more relevant one.
func (r *draftReview) Prepare(d *Documents) { sortEducation(d, r.analysis) }

// Review combines the program's own checks with the fact-checker's report.
func (r *draftReview) Review(d *Documents, rep VerificationReport) findings {
	claims := d.claims()
	issues := map[string]string{}
	for _, c := range claims {
		switch {
		case c.NeedSource && len(c.Sources) == 0:
			issues[c.ID] = "no source quote given"
			continue
		case len(r.prof.Missing(c.Sources)) > 0:
			issues[c.ID] = fmt.Sprintf("quoted source not found in the profile: %q", r.prof.Missing(c.Sources)[0])
			continue
		}
		if c.Ref.Kind == kindEducationDetail && refersToOtherEntry(c.Text) {
			issues[c.ID] = "refers to another entry or to the other degree; every degree entry must stand on its own, so state the fact fully inside this entry or delete the line"
		} else if c.Ref.Kind == kindEducationDetail && courseworkLeadHasRemark(c.Text) {
			issues[c.ID] = "the coursework line must hold only the list of courses; move the remark in parentheses (for example the language of instruction) to its own line"
		}
	}

	seen := map[string]bool{}
	for _, v := range rep.Verdicts {
		if seen[v.ID] {
			continue
		}
		seen[v.ID] = true
		if v.Status != "supported" {
			if prev, ok := issues[v.ID]; ok {
				issues[v.ID] = prev + "; " + v.Status + ": " + v.Reason
			} else {
				issues[v.ID] = v.Status + ": " + v.Reason
			}
		}
	}
	var unreviewed []string
	for _, c := range claims {
		if _, ok := issues[c.ID]; !ok && !seen[c.ID] {
			unreviewed = append(unreviewed, c.ID)
		}
	}

	_, gaps := keywordStatus(r.prof, d, r.analysis, rep.Keywords)
	return findings{Issues: issues, Courses: duplicateCourseIssues(d, r.catalog), Gaps: gaps, Unreviewed: unreviewed, support: rep.Keywords}
}

// problems is the repair note for the writer: every flagged claim, in
// document order.
func (f findings) problems(d *Documents) string {
	var b strings.Builder
	for _, c := range d.claims() {
		var reasons []string
		if r, ok := f.Issues[c.ID]; ok {
			reasons = append(reasons, r)
		}
		if r, ok := f.Courses[c.ID]; ok {
			reasons = append(reasons, r)
		}
		if len(reasons) > 0 {
			fmt.Fprintf(&b, "- %s: %q -> %s\n", c.ID, c.Text, strings.Join(reasons, "; "))
		}
	}
	return b.String()
}

// missingKeywords is the repair note for supported keywords the CV under-uses.
func (f findings) missingKeywords(a JobAnalysis) string { return missingKeywordsList(a, f.Gaps) }

// GiveUp ends the repair rounds: Skills lines are cut down to the items the
// profile lists, the remaining flagged and unreviewed claims are removed, and
// repeated courses deduplicated. Required keywords that the removals took out
// of the CV are reported.
func (r *draftReview) GiveUp(d *Documents, f findings) ([]Dropped, []string) {
	issues := make(map[string]string, len(f.Issues)+len(f.Unreviewed))
	for id, reason := range f.Issues {
		issues[id] = reason
	}
	for _, id := range f.Unreviewed {
		issues[id] = notReviewed
	}
	before := cvText(d)
	dropped := salvageSkills(r.prof, d, issues)
	dropped = append(dropped, drop(d, issues)...)
	warnings := dedupeCoursework(d, r.catalog)
	return dropped, append(warnings, lostKeywords(r.prof, r.analysis, f.support, before, cvText(d))...)
}

// Finalize applies the fixes that need no model: identity, rankings and links
// are checked against the profile, titles and the addressee cleaned up, empty
// Skills and Languages filled from the profile, and the fact-checker's
// coverage of the requirements kept where its evidence is real.
func (r *draftReview) Finalize(d *Documents, rep VerificationReport) []string {
	var w []string
	w = append(w, groundIdentity(r.prof, d)...)
	w = append(w, groundRankings(r.prof, d)...)
	w = append(w, groundProjectLinks(r.prof, d)...)
	cleanEducationTitles(d)
	cleanRecipient(d, r.analysis)
	w = append(w, fillSkillsLanguages(r.prof, d)...)
	// Requirements are assessed by the fact-checker only: it has no stake in the
	// candidate looking good, and the writer has enough to do.
	d.Coverage = rep.Coverage
	groundCoverage(r.prof, d)
	return w
}

// drop removes the claims with issues and lists them.
func drop(docs *Documents, issues map[string]string) []Dropped {
	var out []Dropped
	bad := map[ClaimRef]bool{}
	for _, c := range docs.claims() {
		if reason, ok := issues[c.ID]; ok {
			bad[c.Ref] = true
			out = append(out, Dropped{ID: c.ID, Text: c.Text, Reason: reason})
		}
	}
	docs.prune(bad)
	return out
}

// groundIdentity makes sure the name and contact lines really come from the
// profile; unverifiable contact lines are removed.
func groundIdentity(prof *profile.Profile, docs *Documents) []string {
	var warnings []string
	if !prof.Has(docs.CV.Name) {
		warnings = append(warnings, fmt.Sprintf("name %q was not found in the profile", docs.CV.Name))
	}
	var contact []string
	for _, line := range docs.CV.Contact {
		if prof.Has(line) {
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
func groundCoverage(prof *profile.Profile, docs *Documents) {
	for i, c := range docs.Coverage {
		if c.Covered && !prof.Has(c.Evidence) {
			docs.Coverage[i].Covered = false
			docs.Coverage[i].Evidence = ""
		}
	}
}

var (
	crossReference = regexp.MustCompile(`(?i)\b(same as above|as above|as (?:in|for) the (?:previous|other|first|second)|the (?:previous|other|first|second) degree|both degrees|both of (?:my|the) degrees|likewise|similarly|also)\b`)
)

// refersToOtherEntry reports whether an education line points at another entry
// ("also taught in English", "same as above"), which makes no sense when the
// entries are read on their own.
func refersToOtherEntry(text string) bool { return crossReference.MatchString(text) }

// groundRankings clears a class ranking that is not in the profile (a wrong
// ranking is worse than none) and reports it.
func groundRankings(prof *profile.Profile, docs *Documents) []string {
	var warnings []string
	for i, e := range docs.CV.Education {
		if e.Ranking != "" && !prof.Has(e.Ranking) {
			warnings = append(warnings, fmt.Sprintf("class ranking %q for %q is not in the profile and was removed", e.Ranking, e.Degree))
			docs.CV.Education[i].Ranking = ""
		}
	}
	return warnings
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
