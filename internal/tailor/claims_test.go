package tailor

import "testing"

func fullDocs() Documents {
	c := func(t string) Claim { return Claim{Text: t, Sources: []string{t}} }
	return Documents{
		CV: CV{
			Summary:    c("summary"),
			Skills:     []Claim{c("skill 0"), c("skill 1")},
			Languages:  []Claim{c("lang 0")},
			Projects:   []Project{{Name: "P", Details: []Claim{c("project detail 0"), c("project detail 1")}}},
			Experience: []Experience{{Title: "E0", Bullets: []Claim{c("bullet 0")}}, {Title: "E1", Bullets: []Claim{c("bullet 1")}}},
			Education:  []Education{{Degree: "D", Details: []Claim{c("edu detail 0")}}},
			Sections:   []Section{{Title: "S", Items: []Claim{c("item 0")}}},
		},
		Letter: Letter{Paragraphs: []Claim{c("para 0"), c("para 1")}},
	}
}

// Removing one claim by its ref removes that claim and nothing else; removing
// a header removes its whole entry.
func TestPruneRemovesExactlyTheRef(t *testing.T) {
	all := fullDocs()
	claims := all.claims()
	ids := map[string]bool{}
	for _, c := range claims {
		if ids[c.ID] {
			t.Fatalf("duplicate id %s", c.ID)
		}
		ids[c.ID] = true
	}
	children := map[claimKind]int{kindProject: 2, kindExperience: 1, kindEducation: 1}
	for _, c := range claims {
		d := fullDocs()
		d.prune(map[ClaimRef]bool{c.Ref: true})
		left := d.claims()
		want := len(claims) - 1 - children[c.Ref.Kind]
		if c.Ref.Kind == kindSectionItem {
			want = len(claims) - 1 // the emptied section goes with it
		}
		if len(left) != want {
			t.Errorf("%s: %d claims left, want %d", c.ID, len(left), want)
		}
		for _, l := range left {
			if l.Text == c.Text {
				t.Errorf("%s: %q still present", c.ID, c.Text)
			}
		}
	}
}
