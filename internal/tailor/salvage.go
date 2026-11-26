package tailor

import (
	"fmt"
	"strings"

	"github.com/BernatRubio/best-candidate/internal/profile"
)

// salvageSkills rescues the flagged Skills lines that can be repaired without
// the model. A Skills line is a list, and one unsupported item ("Security"
// added to "Linux/Bash, Wireshark, ...") would otherwise take the whole line
// with it. Each flagged line keeps only the items that the profile's Skills
// section lists word for word; the line's issue is cleared and the items taken
// out are returned for the report. A line that would lose nothing (the problem
// is elsewhere, such as its label) or keep nothing is left to be dropped.
func salvageSkills(prof *profile.Profile, d *Documents, issues map[string]string) []Dropped {
	listed := map[string]string{} // normalized item -> profile line holding it
	for _, it := range prof.Skills() {
		for _, s := range profile.SplitList(it.Value) {
			if n := normalize(s); n != "" {
				listed[n] = it.Line
			}
		}
	}
	var out []Dropped
	for _, c := range d.claims() {
		reason, flagged := issues[c.ID]
		if !flagged || c.Ref.Kind != kindSkill {
			continue
		}
		line := &d.CV.Skills[c.Ref.Line]
		label, value, ok := strings.Cut(line.Text, ": ")
		if !ok || strings.TrimSpace(label) == "" {
			continue
		}
		var kept, removed, sources []string
		seen := map[string]bool{}
		for _, item := range profile.SplitList(value) {
			if item == "" {
				continue
			}
			src, ok := listed[normalize(item)]
			if !ok {
				removed = append(removed, item)
				continue
			}
			kept = append(kept, item)
			if !seen[src] {
				seen[src] = true
				sources = append(sources, src)
			}
		}
		if len(kept) == 0 || len(removed) == 0 {
			continue
		}
		line.Text = strings.TrimSpace(label) + ": " + strings.Join(kept, ", ")
		line.Sources = sources
		delete(issues, c.ID)
		out = append(out, Dropped{
			ID:     c.ID,
			Text:   strings.Join(removed, ", "),
			Reason: fmt.Sprintf("%s (only this was removed; the rest of the line is kept as the profile's Skills section lists it)", reason),
		})
	}
	return out
}

// lostKeywords warns about the required keywords that the profile supports and
// that the CV no longer names after the removals, so that the user can put
// them back by hand.
func lostKeywords(prof *profile.Profile, a JobAnalysis, support []KeywordSupport, before, after string) []string {
	ok := map[string]bool{}
	for _, s := range support {
		if s.Supported && prof.Has(s.Evidence) {
			ok[normalize(s.Term)] = true
		}
	}
	var w []string
	for _, k := range a.Keywords {
		if k.Importance == ImportanceRequired && ok[normalize(k.Term)] && appearsIn(before, k) && !appearsIn(after, k) {
			w = append(w, fmt.Sprintf("the required keyword %q, which your profile supports, is no longer in the CV after the unverified statements were removed; add it back by hand if it is true", k.Term))
		}
	}
	return w
}
