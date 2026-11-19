package tailor

import (
	"sort"
	"strings"
)

// sortEducation lists the degrees from the most to the least relevant to the
// job. The model scores each degree (Relevance, 0 to 10); ties are broken by how
// many of the offer's keywords the degree entry mentions, then by the order the
// model wrote them in. Ordering is done here, not left to the model, because
// models fall back to the order of the profile.
//
// It runs right after every tailoring call, before the checks, so that the
// positional claim ids stay valid and so that a course shared by two degrees is
// kept under the more relevant one (the first entry keeps a repeated course).
func sortEducation(docs *Documents, a JobAnalysis) {
	edu := docs.CV.Education
	if len(edu) < 2 {
		return
	}
	var keywords []string
	for _, group := range [][]string{a.Keywords, a.MustHave} {
		for _, k := range group {
			if n := normalize(k); n != "" {
				keywords = append(keywords, n)
			}
		}
	}
	score := make([]int, len(edu))
	for i, e := range edu {
		text := e.Degree
		for _, d := range e.Details {
			text += " " + d.Text
		}
		hay := " " + normalize(text) + " "
		for _, k := range keywords {
			if strings.Contains(hay, " "+k+" ") {
				score[i]++
			}
		}
	}
	idx := make([]int, len(edu))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(x, y int) bool {
		i, j := idx[x], idx[y]
		if edu[i].Relevance != edu[j].Relevance {
			return edu[i].Relevance > edu[j].Relevance
		}
		return score[i] > score[j]
	})
	sorted := make([]Education, len(edu))
	for pos, i := range idx {
		sorted[pos] = edu[i]
	}
	docs.CV.Education = sorted
}
