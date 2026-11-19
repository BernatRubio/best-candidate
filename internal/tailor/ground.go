package tailor

import (
	"strings"
	"unicode"
)

// normalize reduces text to lowercase word tokens so that a quote still matches
// when the model drops markdown markup, changes punctuation or re-wraps lines.
// Letters, digits and + # @ / % are kept (c++, c#, e-mail addresses, urls,
// percentages); everything else separates tokens.
func normalize(s string) string {
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

// grounder checks quotes against the profile text.
type grounder struct {
	profile string // normalized, padded with spaces
}

func newGrounder(profile string) *grounder {
	return &grounder{profile: " " + normalize(profile) + " "}
}

// has reports whether quote appears in the profile as a run of whole tokens.
func (g *grounder) has(quote string) bool {
	q := normalize(quote)
	if q == "" {
		return false
	}
	return strings.Contains(g.profile, " "+q+" ")
}

// missing returns the quotes that are not in the profile.
func (g *grounder) missing(quotes []string) []string {
	var out []string
	for _, q := range quotes {
		if !g.has(q) {
			out = append(out, q)
		}
	}
	return out
}
