package tailor

import (
	"fmt"
	"strings"

	"github.com/BernatRubio/best-candidate/internal/profile"
)

// ATS is how well the CV matches the offer's keywords. Applicant tracking
// systems mostly match literal words, so a keyword the profile supports but the
// CV leaves out is a lost match; a keyword the profile does not show is not
// something the tool may add.
type ATS struct {
	Present      []string // supported by the profile and in the CV
	Missing      []string // supported by the profile but not in the CV
	NotInBullets []string // in the CV only as a skill, though the profile shows it used in work
	NotInProfile []string // in the offer, not shown by the profile
	Bullets      int      // experience and project bullets in the CV
	Unchanged    int      // of those, bullets identical to the profile line they quote
}

// keywordsTag lists the offer's keywords for the fact-checker.
func keywordsTag(a JobAnalysis) string {
	var b strings.Builder
	for _, k := range a.Keywords {
		if strings.TrimSpace(k.Term) == "" {
			continue
		}
		fmt.Fprintf(&b, "%s", k.Term)
		if len(k.Variants) > 0 {
			fmt.Fprintf(&b, " (also written: %s)", strings.Join(k.Variants, ", "))
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return ""
	}
	return tag("keywords", b.String())
}

// cvText is every sentence of the CV that an ATS reads, normalized and padded
// so that whole tokens can be searched.
func cvText(cv *CV) string {
	parts := []string{cv.Summary.Text}
	for _, c := range cv.Skills {
		parts = append(parts, c.Text)
	}
	for _, p := range cv.Projects {
		parts = append(parts, p.Name)
		for _, b := range p.Details {
			parts = append(parts, b.Text)
		}
	}
	for _, e := range cv.Experience {
		parts = append(parts, e.Title)
		for _, b := range e.Bullets {
			parts = append(parts, b.Text)
		}
	}
	for _, e := range cv.Education {
		parts = append(parts, e.Degree)
		for _, b := range e.Details {
			parts = append(parts, b.Text)
		}
	}
	for _, s := range cv.Sections {
		for _, b := range s.Items {
			parts = append(parts, b.Text)
		}
	}
	return " " + normalize(strings.Join(parts, " ")) + " "
}

// bulletText is the experience and project bullets only, normalized like cvText.
func bulletText(cv *CV) string {
	var parts []string
	for _, p := range cv.Projects {
		for _, b := range p.Details {
			parts = append(parts, b.Text)
		}
	}
	for _, e := range cv.Experience {
		for _, b := range e.Bullets {
			parts = append(parts, b.Text)
		}
	}
	return " " + normalize(strings.Join(parts, " ")) + " "
}

// countBullets returns how many experience and project bullets the CV has and
// how many of them are word for word one of the profile lines they cite: a
// sign that nothing was tailored to the offer.
func countBullets(cv *CV) (total, unchanged int) {
	count := func(c Claim) {
		total++
		for _, src := range c.Sources {
			if normalize(src) == normalize(c.Text) {
				unchanged++
				return
			}
		}
	}
	for _, p := range cv.Projects {
		for _, b := range p.Details {
			count(b)
		}
	}
	for _, e := range cv.Experience {
		for _, b := range e.Bullets {
			count(b)
		}
	}
	return total, unchanged
}

func appearsIn(hay string, k KeyTerm) bool {
	for _, f := range k.forms() {
		if n := normalize(f); n != "" && strings.Contains(hay, " "+n+" ") {
			return true
		}
	}
	return false
}

// keywordGap is a supported keyword the CV should say more clearly.
type keywordGap struct {
	Evidence string
	InCV     bool // already in the CV, but not in any bullet
}

// keywordStatus sorts the offer's keywords into present, missing and not in
// the profile. Only the fact-checker's verdict, confirmed by a verbatim quote,
// makes a keyword "supported"; without a verdict nothing is reported missing.
//
// A keyword that is in the CV only as a skill is also a gap when the profile
// shows it used in work (its evidence is outside the Skills section): the CV
// then has a bullet it can name the keyword in. A keyword whose only evidence
// is the profile's Skills section cannot be placed in a bullet truthfully, so
// it is left alone.
func keywordStatus(prof *profile.Profile, docs *Documents, a JobAnalysis, support []KeywordSupport) (ATS, map[string]keywordGap) {
	var out ATS
	out.Bullets, out.Unchanged = countBullets(&docs.CV)
	gaps := map[string]keywordGap{}
	supported := map[string]string{}
	for _, s := range support {
		if s.Supported && prof.Has(s.Evidence) {
			supported[normalize(s.Term)] = s.Evidence
		}
	}
	hay, bullets := cvText(&docs.CV), bulletText(&docs.CV)
	for _, k := range a.Keywords {
		ev, ok := supported[normalize(k.Term)]
		switch {
		case !ok && len(support) > 0:
			out.NotInProfile = append(out.NotInProfile, k.Term)
		case !ok:
			// no verdict at all: say nothing
		case appearsIn(hay, k):
			out.Present = append(out.Present, k.Term)
			onlySkill := prof.InSkills(ev)
			if !appearsIn(bullets, k) && !onlySkill && out.Bullets > 0 {
				out.NotInBullets = append(out.NotInBullets, k.Term)
				gaps[k.Term] = keywordGap{Evidence: ev, InCV: true}
			}
		default:
			out.Missing = append(out.Missing, k.Term)
			gaps[k.Term] = keywordGap{Evidence: ev}
		}
	}
	return out, gaps
}

// missingKeywordsList is the repair note for supported keywords that the CV
// leaves out, in the offer's order.
func missingKeywordsList(a JobAnalysis, gaps map[string]keywordGap) string {
	var b strings.Builder
	for _, k := range a.Keywords {
		if gap, ok := gaps[k.Term]; ok {
			note := "not in the CV"
			if gap.InCV {
				note = "in the CV only as a skill, in no bullet"
			}
			fmt.Fprintf(&b, "- %s [%s] (profile evidence: %q)\n", strings.Join(k.forms(), " / "), note, gap.Evidence)
		}
	}
	return b.String()
}
