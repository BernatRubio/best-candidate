package tailor

import (
	"fmt"
	"regexp"
	"strings"
)

// KeywordHit is an offer keyword and how many times the CV uses it.
type KeywordHit struct {
	Keyword
	Count int
}

// Coverage counts how many times the CV uses each keyword.
func Coverage(kws []Keyword, cv CV) []KeywordHit {
	text := cvText(cv)
	hits := make([]KeywordHit, 0, len(kws))
	for _, k := range kws {
		hits = append(hits, KeywordHit{Keyword: k, Count: count(text, k.Term)})
	}
	return hits
}

// MissingWarning is the warning for a supported keyword the CV leaves out.
func MissingWarning(h KeywordHit) string {
	return fmt.Sprintf("keyword %q is supported by the profile but missing from the CV", h.Term)
}

// cvText joins every piece of CV text an applicant tracking system would read.
func cvText(cv CV) string {
	var parts []string
	add := func(cs []Claim) {
		for _, c := range cs {
			parts = append(parts, c.Text)
		}
	}
	parts = append(parts, cv.Summary.Text)
	add(cv.Skills)
	add(cv.Languages)
	for _, e := range cv.Experience {
		parts = append(parts, e.Title)
		add(e.Bullets)
	}
	for _, p := range cv.Projects {
		parts = append(parts, p.Name)
		add(p.Details)
	}
	for _, e := range cv.Education {
		parts = append(parts, e.Degree)
		add(e.Details)
	}
	for _, s := range cv.Sections {
		add(s.Items)
	}
	return strings.Join(parts, "\n")
}

// count returns how many times term occurs in text, ignoring case, as a whole
// word: the characters next to it must not be letters or digits. This works
// for terms such as C++ or CI/CD, where \b would not.
func count(text, term string) int {
	term = strings.TrimSpace(term)
	if term == "" {
		return 0
	}
	re := regexp.MustCompile(`(?i)(?:^|[^\pL\pN])(` + regexp.QuoteMeta(term) + `)`)
	n := 0
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		end := m[3]
		if end < len(text) && isWordRune(text[end:]) {
			continue
		}
		n++
	}
	return n
}

var wordStart = regexp.MustCompile(`^[\pL\pN]`)

func isWordRune(s string) bool { return wordStart.MatchString(s) }
