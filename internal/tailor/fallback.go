package tailor

import (
	"fmt"
	"strings"
)

// Weaker models skip parts of the schema they are asked to fill. Skills and
// languages are plain facts the profile already states, so when the model
// leaves them empty they are taken from the profile mechanically, each with its
// exact profile line as evidence. This keeps the CV complete; the relevance
// ordering a good model would add is lost, and the report says so.

// Section headings the profile may use, in English, Spanish and Catalan,
// matched as prefixes of the lower-cased heading.
var (
	skillsHeadings    = []string{"skills", "habilidades", "habilitats", "competencias", "competències"}
	languagesHeadings = []string{"languages", "idiomas", "idiomes"}
	educationHeadings = []string{"education", "formación", "formació", "educación", "educació", "estudios", "estudis"}
)

// skillsText is the profile's Skills section, normalized and padded with
// spaces so that whole tokens can be searched.
func skillsText(profile string) string {
	return " " + normalize(strings.Join(profileSection(profile, skillsHeadings...), "\n")) + " "
}

// profileSection returns the trimmed lines under the first "## <name>" heading
// whose title starts with one of names (case-insensitive).
func profileSection(profile string, names ...string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(profile, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") {
			if in {
				break
			}
			title := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(t, "## ")))
			for _, n := range names {
				if strings.HasPrefix(title, n) {
					in = true
				}
			}
			continue
		}
		if in {
			out = append(out, t)
		}
	}
	return out
}

// labelledBullets returns the "- Label: value" bullets of a profile section as
// (label, value, whole line without the dash).
func labelledBullets(lines []string) [][3]string {
	var out [][3]string
	for _, l := range lines {
		if !strings.HasPrefix(l, "- ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(l, "- "))
		i := strings.Index(rest, ": ")
		if i <= 0 || strings.TrimSpace(rest[i+2:]) == "" {
			continue
		}
		out = append(out, [3]string{strings.TrimSpace(rest[:i]), strings.TrimSpace(rest[i+2:]), rest})
	}
	return out
}

var levelText = strings.NewReplacer(" (", ", ", "(", ", ", ")", "")

// profileSkills returns one claim per "Category: a, b, c" bullet under the
// profile's Skills section.
func profileSkills(profile string) []Claim {
	var out []Claim
	for _, b := range labelledBullets(profileSection(profile, skillsHeadings...)) {
		out = append(out, Claim{Text: b[2], Sources: []string{b[2]}})
	}
	return out
}

// profileLanguages returns one "Name (level)" claim per "Language: level"
// bullet under the profile's Languages section.
func profileLanguages(profile string) []Claim {
	var out []Claim
	for _, b := range labelledBullets(profileSection(profile, languagesHeadings...)) {
		if len(strings.Fields(b[0])) > 3 {
			continue // a remark such as "Studied both degrees: ..." is not a language
		}
		out = append(out, Claim{Text: b[0] + " (" + levelText.Replace(b[1]) + ")", Sources: []string{b[2]}})
	}
	return out
}

// fillSkillsLanguages completes an empty Skills or Languages block from the
// profile and returns a warning for each block it had to fill.
func fillSkillsLanguages(profile string, docs *Documents) []string {
	var warnings []string
	if len(docs.CV.Skills) == 0 {
		if skills := profileSkills(profile); len(skills) > 0 {
			docs.CV.Skills = skills
			warnings = append(warnings, "the model wrote no Skills block, so all skills from the profile were added as they are (not sorted by relevance to the job)")
		}
	}
	if len(docs.CV.Languages) == 0 {
		if langs := profileLanguages(profile); len(langs) > 0 {
			docs.CV.Languages = langs
			warnings = append(warnings, "the model wrote no Languages block, so the languages from the profile were added")
		}
	}
	return warnings
}

// groundProjectLinks clears a project link that is not in the profile and
// reports it; a made-up URL is worse than none.
func groundProjectLinks(g *grounder, docs *Documents) []string {
	var warnings []string
	for i, p := range docs.CV.Projects {
		if p.Link != "" && !g.has(p.Link) {
			warnings = append(warnings, fmt.Sprintf("link %q for project %q is not in the profile and was removed", p.Link, p.Name))
			docs.CV.Projects[i].Link = ""
		}
	}
	return warnings
}
