package profile

import (
	"strings"
	"testing"
)

func TestHasMatchesWholeTokens(t *testing.T) {
	p := Parse("- **Built** REST APIs in Go,\n  for the billing team\n- C++ and C#")
	for _, q := range []string{"built rest apis in go for the billing team", "Built REST APIs in Go", "C++", "C#"} {
		if !p.Has(q) {
			t.Errorf("expected match for %q", q)
		}
	}
	for _, q := range []string{"Led the billing team", "REST AP", "", "go for billing"} {
		if p.Has(q) {
			t.Errorf("unexpected match for %q", q)
		}
	}
	if got := p.Missing([]string{"C++", "Rust"}); len(got) != 1 || got[0] != "Rust" {
		t.Errorf("missing = %v", got)
	}
}

func TestHasIgnoresComments(t *testing.T) {
	if Parse("# Ada\n<!-- led a team of 10 -->\n- Used Go").Has("led a team of 10") {
		t.Fatal("a comment must not count as evidence")
	}
}

func TestSplitList(t *testing.T) {
	got := SplitList("Parallel Programming (OpenMP, CUDA), Databases, Networks")
	if len(got) != 3 || got[0] != "Parallel Programming (OpenMP, CUDA)" || got[2] != "Networks" {
		t.Fatalf("got %q", got)
	}
}

// Section headings are read in English, Spanish and Catalan.
func TestSectionsUnderTranslatedHeadings(t *testing.T) {
	for _, h := range [][3]string{
		{"Skills", "Languages", "Education"},
		{"Habilidades", "Idiomas", "Formación"},
		{"Habilitats", "Idiomes", "Formació"},
	} {
		p := Parse("# Ada\n\n## " + h[0] + "\n\n- Tools: Git, Docker\n\n## " + h[1] +
			"\n\n- English: B2\n- Studied both degrees entirely in English: yes\n\n## " + h[2] +
			"\n\n### Degree, UPF (2020 - 2025)\n- Systems: Operating Systems, Databases, Networks\n")
		if s := p.Skills(); len(s) != 1 || s[0].Line != "Tools: Git, Docker" {
			t.Errorf("%s: skills = %+v", h[0], s)
		}
		if l := p.Languages(); len(l) != 1 || l[0].Label != "English" || l[0].Value != "B2" {
			t.Errorf("%s: languages = %+v", h[1], l)
		}
		if c := p.Courses(); strings.Join(c, ",") != "operating systems,databases,networks" {
			t.Errorf("%s: courses = %v", h[2], c)
		}
		if !p.InSkills("Docker") || p.InSkills("Operating Systems") {
			t.Errorf("%s: InSkills wrong", h[0])
		}
	}
}
