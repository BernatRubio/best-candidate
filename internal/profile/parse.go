package profile

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Profile is a parsed master profile. The text is sent to the model as it is;
// the parsed parts answer the questions the tool asks about it: is this quote
// in the profile, what skills, languages and courses does it list.
type Profile struct {
	text      string
	norm      string // Normalize(text), padded with spaces
	skills    []string
	languages []string
	education []string
}

// Item is one "- Label: value" bullet of a profile section.
type Item struct {
	Label string
	Value string
	Line  string // the bullet without its dash, as the profile writes it
}

// Section headings the profile may use, in English, Spanish and Catalan,
// matched as prefixes of the lower-cased "## " heading.
var (
	skillsHeadings    = []string{"skills", "habilidades", "habilitats", "competencias", "competències"}
	languagesHeadings = []string{"languages", "idiomas", "idiomes"}
	educationHeadings = []string{"education", "formación", "formació", "educación", "educació", "estudios", "estudis"}
)

// Parse cleans the text (see Clean) and reads its sections.
func Parse(text string) *Profile {
	text = Clean(text)
	return &Profile{
		text:      text,
		norm:      " " + Normalize(text) + " ",
		skills:    section(text, skillsHeadings),
		languages: section(text, languagesHeadings),
		education: section(text, educationHeadings),
	}
}

// Text is the cleaned profile, as the model sees it.
func (p *Profile) Text() string { return p.text }

// Has reports whether quote appears in the profile as a run of whole tokens,
// whatever the markup, punctuation or line breaks around it.
func (p *Profile) Has(quote string) bool {
	q := Normalize(quote)
	return q != "" && strings.Contains(p.norm, " "+q+" ")
}

// Missing returns the quotes that are not in the profile.
func (p *Profile) Missing(quotes []string) []string {
	var out []string
	for _, q := range quotes {
		if !p.Has(q) {
			out = append(out, q)
		}
	}
	return out
}

// InSkills reports whether quote lies inside the Skills section, as opposed to
// the description of some piece of work.
func (p *Profile) InSkills(quote string) bool {
	q := Normalize(quote)
	return q != "" && strings.Contains(" "+Normalize(strings.Join(p.skills, "\n"))+" ", " "+q+" ")
}

// Skills returns the "- Category: a, b, c" bullets of the Skills section.
func (p *Profile) Skills() []Item { return items(p.skills) }

// Languages returns the "- Language: level" bullets of the Languages section.
// Remarks written as bullets ("- Studied both degrees: ...") are left out.
func (p *Profile) Languages() []Item {
	var out []Item
	for _, it := range items(p.languages) {
		if len(strings.Fields(it.Label)) <= 3 {
			out = append(out, it)
		}
	}
	return out
}

var gradeLike = regexp.MustCompile(`\d\.\d`)

// Courses returns the course names listed in the Education section, as
// "- Area: course, course, course" bullets, each reduced by CourseKey. They
// are ordered longest first, so that "advanced computer graphics" is matched
// before "computer graphics". Lines with fewer than three items or with grades
// are not course lists.
func (p *Profile) Courses() []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items(p.education) {
		list := SplitList(it.Value)
		if len(list) < 3 {
			continue
		}
		for _, c := range list {
			k := CourseKey(c)
			if len(k) < 4 || gradeLike.MatchString(c) || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if len(out[a]) != len(out[b]) {
			return len(out[a]) > len(out[b])
		}
		return out[a] < out[b]
	})
	return out
}

var parenthetical = regexp.MustCompile(`\([^)]*\)`)

// CourseKey reduces a course name to comparable tokens: parentheticals, a
// leading "and" and punctuation are dropped.
func CourseKey(s string) string {
	k := Normalize(parenthetical.ReplaceAllString(s, " "))
	return strings.TrimSpace(strings.TrimPrefix(k, "and "))
}

// StripParentheticals removes every "(...)" from s.
func StripParentheticals(s string) string { return parenthetical.ReplaceAllString(s, " ") }

// SplitList splits on commas that are not inside parentheses, so
// "Parallel Programming (OpenMP, CUDA), Databases" gives two items.
func SplitList(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// Normalize reduces text to lowercase word tokens so that a quote still matches
// when the model drops markdown markup, changes punctuation or re-wraps lines.
// Letters, digits and + # @ / % are kept (c++, c#, e-mail addresses, urls,
// percentages); everything else separates tokens.
func Normalize(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("+#@/%", r):
			b.WriteRune(r)
			space = false
		default:
			if !space {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// section returns the trimmed lines under the first "## " heading whose title
// starts with one of names (case-insensitive).
func section(text string, names []string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(text, "\n") {
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

// items returns the "- Label: value" bullets among lines.
func items(lines []string) []Item {
	var out []Item
	for _, l := range lines {
		if !strings.HasPrefix(l, "- ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(l, "- "))
		i := strings.Index(rest, ": ")
		if i <= 0 || strings.TrimSpace(rest[i+2:]) == "" {
			continue
		}
		out = append(out, Item{Label: strings.TrimSpace(rest[:i]), Value: strings.TrimSpace(rest[i+2:]), Line: rest})
	}
	return out
}
