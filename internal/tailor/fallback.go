package tailor

import (
	"fmt"
	"strings"

	"github.com/BernatRubio/best-candidate/internal/profile"
)

// Weaker models skip parts of the schema they are asked to fill. Skills and
// languages are plain facts the profile already states, so when the model
// leaves them empty they are taken from the profile mechanically, each with its
// exact profile line as evidence. This keeps the CV complete; the relevance
// ordering a good model would add is lost, and the report says so.

var levelText = strings.NewReplacer(" (", ", ", "(", ", ", ")", "")

// profileSkills returns one claim per "Category: a, b, c" bullet under the
// profile's Skills section.
func profileSkills(prof *profile.Profile) []Claim {
	var out []Claim
	for _, it := range prof.Skills() {
		out = append(out, Claim{Text: it.Line, Sources: []string{it.Line}})
	}
	return out
}

// profileLanguages returns one "Name (level)" claim per "Language: level"
// bullet under the profile's Languages section.
func profileLanguages(prof *profile.Profile) []Claim {
	var out []Claim
	for _, it := range prof.Languages() {
		out = append(out, Claim{Text: it.Label + " (" + levelText.Replace(it.Value) + ")", Sources: []string{it.Line}})
	}
	return out
}

// fillSkillsLanguages completes an empty Skills or Languages block from the
// profile and returns a warning for each block it had to fill.
func fillSkillsLanguages(prof *profile.Profile, docs *Documents) []string {
	var warnings []string
	if len(docs.CV.Skills) == 0 {
		if skills := profileSkills(prof); len(skills) > 0 {
			docs.CV.Skills = skills
			warnings = append(warnings, "the model wrote no Skills block, so all skills from the profile were added as they are (not sorted by relevance to the job)")
		}
	}
	if len(docs.CV.Languages) == 0 {
		if langs := profileLanguages(prof); len(langs) > 0 {
			docs.CV.Languages = langs
			warnings = append(warnings, "the model wrote no Languages block, so the languages from the profile were added")
		}
	}
	return warnings
}

// groundProjectLinks clears a project link that is not in the profile and
// reports it; a made-up URL is worse than none.
func groundProjectLinks(prof *profile.Profile, docs *Documents) []string {
	var warnings []string
	for i, p := range docs.CV.Projects {
		if p.Link != "" && !prof.Has(p.Link) {
			warnings = append(warnings, fmt.Sprintf("link %q for project %q is not in the profile and was removed", p.Link, p.Name))
			docs.CV.Projects[i].Link = ""
		}
	}
	return warnings
}
