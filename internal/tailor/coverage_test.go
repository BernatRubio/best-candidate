package tailor

import "testing"

func TestCount(t *testing.T) {
	for _, tc := range []struct {
		text, term string
		want       int
	}{
		{"Built APIs in Go. Go again.", "Go", 2},
		{"Worked at Google on Golang tooling", "Go", 0}, // whole word only
		{"used postgresql daily", "PostgreSQL", 1},      // case-insensitive
		{"C++ and C, not C++11", "C++", 1},
		{"Wrote C# services", "C#", 1},
		{"Set up CI/CD pipelines", "CI/CD", 1},
		{"Built REST APIs with REST principles", "REST APIs", 1},
		{"Machine Learning (ML)", "ML", 1},
		{"anything", "  ", 0},
	} {
		if got := count(tc.text, tc.term); got != tc.want {
			t.Errorf("count(%q, %q) = %d, want %d", tc.text, tc.term, got, tc.want)
		}
	}
}

func TestCoverageReadsEveryCVPart(t *testing.T) {
	cv := CV{
		Summary:    Claim{Text: "summaryword"},
		Skills:     []Claim{{Text: "skillword"}},
		Experience: []Experience{{Bullets: []Claim{{Text: "bulletword"}}}},
		Projects:   []Project{{Details: []Claim{{Text: "projectword"}}}},
		Education:  []Education{{Details: []Claim{{Text: "courseword"}}}},
		Sections:   []Section{{Items: []Claim{{Text: "sectionword"}}}},
	}
	var kws []Keyword
	for _, w := range []string{"summaryword", "skillword", "bulletword", "projectword", "courseword", "sectionword"} {
		kws = append(kws, Keyword{Term: w})
	}
	for _, h := range Coverage(kws, cv) {
		if h.Count != 1 {
			t.Errorf("%s: count = %d, want 1", h.Term, h.Count)
		}
	}
}
