package tailor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/BernatRubio/best-candidate/internal/llm"
)

const testProfile = `# Ada Example
ada@example.com

## Experience
### Backend developer, Initech (2020-2023)
- Built REST APIs in Go for the billing team
- Helped migrate the CI pipeline to GitHub Actions

## Skills
Go, PostgreSQL, Docker
`

const testOffer = "Acme is hiring a Go developer. Requirements: Go, Kubernetes."

var analysis = JobAnalysis{
	Company: "Acme", Role: "Go developer", Language: "en",
	MustHave: []string{"Go", "Kubernetes"}, Keywords: []string{"Go"}, Tone: "technical",
}

func baseDocs() Documents {
	return Documents{
		CV: CV{
			Name:    "Ada Example",
			Contact: []string{"ada@example.com", "+34 600 000 000"},
			Experience: []Experience{{
				Title: "Backend developer", Company: "Initech", Period: "2020-2023",
				Sources: []string{"Backend developer, Initech (2020-2023)"},
				Bullets: []Claim{
					{Text: "Built REST APIs in Go for the billing team", Sources: []string{"Built REST APIs in Go for the billing team"}},
				},
			}},
			Sections: []Section{{Title: "Skills", Layout: LayoutInline, Placement: PlaceAfterEducation, Items: []Claim{
				{Text: "Go", Sources: []string{"Go, PostgreSQL, Docker"}},
			}}},
		},
		Letter: Letter{
			Greeting: "Dear hiring team,",
			Paragraphs: []Claim{
				{Text: "At Initech I built REST APIs in Go for the billing team.", Sources: []string{"Built REST APIs in Go for the billing team"}},
			},
			Closing: "Kind regards,", Signature: "Ada Example",
		},
		Coverage: []Coverage{
			{Requirement: "Go", Covered: true, Evidence: "Built REST APIs in Go"},
			{Requirement: "Kubernetes", Covered: true, Evidence: "Ran Kubernetes clusters"},
		},
	}
}

func allSupported(docs Documents) func(llm.Request) (json.RawMessage, error) {
	return func(llm.Request) (json.RawMessage, error) {
		var r VerificationReport
		for _, c := range docs.claims() {
			r.Verdicts = append(r.Verdicts, Verdict{ID: c.ID, Status: "supported", Reason: "ok"})
		}
		return json.Marshal(r)
	}
}

func TestRunCleanPath(t *testing.T) {
	docs := baseDocs()
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), allSupported(docs))

	res, err := Run(context.Background(), fake, testProfile, testOffer, Options{MaxRepairs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 3 {
		t.Fatalf("want 3 model calls, got %d", len(fake.Calls))
	}
	if len(res.Dropped) != 0 {
		t.Fatalf("nothing should be dropped: %+v", res.Dropped)
	}
	if res.Lang != "en" {
		t.Fatalf("lang = %q", res.Lang)
	}
	// Identity: the invented phone number is not in the profile and must go.
	if got := res.Docs.CV.Contact; len(got) != 1 || got[0] != "ada@example.com" {
		t.Fatalf("contact = %v", got)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "+34 600") {
		t.Fatalf("warnings = %v", res.Warnings)
	}
	// Coverage: "Kubernetes" evidence is fabricated, so it must be downgraded.
	for _, c := range res.Docs.Coverage {
		if c.Requirement == "Kubernetes" && c.Covered {
			t.Fatal("Kubernetes must not count as covered")
		}
		if c.Requirement == "Go" && !c.Covered {
			t.Fatal("Go should be covered")
		}
	}
}

// The model keeps a fabricated bullet whose quote is not in the profile, even
// after a repair round and even though the model reviewer waves it through.
// It must be dropped while the honest bullet survives.
func TestRunDropsFabricatedClaim(t *testing.T) {
	docs := baseDocs()
	docs.CV.Experience[0].Bullets = append(docs.CV.Experience[0].Bullets, Claim{
		Text:    "Led a team of 10 engineers",
		Sources: []string{"Led a team of 10 engineers"},
	})
	fake := llm.NewFake(
		llm.Reply(analysis),
		llm.Reply(docs), allSupported(docs), // round 1: reviewer is fooled, quote check is not
		llm.Reply(docs), allSupported(docs), // repair returns the same lie
	)

	res, err := Run(context.Background(), fake, testProfile, testOffer, Options{MaxRepairs: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dropped) != 1 || res.Dropped[0].ID != "cv.experience[0].bullets[1]" {
		t.Fatalf("dropped = %+v", res.Dropped)
	}
	bullets := res.Docs.CV.Experience[0].Bullets
	if len(bullets) != 1 || !strings.Contains(bullets[0].Text, "REST APIs") {
		t.Fatalf("bullets = %+v", bullets)
	}
	// The repair call must have been told what was wrong.
	repair := fake.Calls[3].User
	if !strings.Contains(repair, "<problems>") || !strings.Contains(repair, "Led a team of 10") {
		t.Fatalf("repair prompt lacks problems:\n%s", repair)
	}
}

// An exaggeration caught by the reviewer is fixed by a repair round, so
// nothing needs to be dropped.
func TestRunRepairsExaggeration(t *testing.T) {
	bad := baseDocs()
	bad.CV.Experience[0].Bullets = append(bad.CV.Experience[0].Bullets, Claim{
		Text:    "Led the migration of the CI pipeline to GitHub Actions",
		Sources: []string{"Helped migrate the CI pipeline to GitHub Actions"},
	})
	flag := func(llm.Request) (json.RawMessage, error) {
		var r VerificationReport
		for _, c := range bad.claims() {
			v := Verdict{ID: c.ID, Status: "supported", Reason: "ok"}
			if strings.HasPrefix(c.Text, "Led the migration") {
				v = Verdict{ID: c.ID, Status: "exaggerated", Reason: "profile says helped, not led"}
			}
			r.Verdicts = append(r.Verdicts, v)
		}
		return json.Marshal(r)
	}

	fixed := baseDocs()
	fixed.CV.Experience[0].Bullets = append(fixed.CV.Experience[0].Bullets, Claim{
		Text:    "Helped migrate the CI pipeline to GitHub Actions",
		Sources: []string{"Helped migrate the CI pipeline to GitHub Actions"},
	})

	fake := llm.NewFake(
		llm.Reply(analysis),
		llm.Reply(bad), flag,
		llm.Reply(fixed), allSupported(fixed),
	)
	res, err := Run(context.Background(), fake, testProfile, testOffer, Options{MaxRepairs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dropped) != 0 {
		t.Fatalf("dropped = %+v", res.Dropped)
	}
	got := res.Docs.CV.Experience[0].Bullets[1].Text
	if !strings.HasPrefix(got, "Helped migrate") {
		t.Fatalf("bullet = %q", got)
	}
}

func TestLangOverride(t *testing.T) {
	docs := baseDocs()
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), allSupported(docs))
	res, err := Run(context.Background(), fake, testProfile, testOffer, Options{Lang: "es"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Lang != "es" || !strings.Contains(fake.Calls[1].User, "<language>\nes\n</language>") {
		t.Fatalf("lang = %q", res.Lang)
	}
}

func TestNormalizeQuoteMatching(t *testing.T) {
	g := newGrounder("- **Built** REST APIs in Go,\n  for the billing team\n- C++ and C#")
	for _, q := range []string{"built rest apis in go for the billing team", "Built REST APIs in Go", "C++", "C#"} {
		if !g.has(q) {
			t.Errorf("expected match for %q", q)
		}
	}
	for _, q := range []string{"Led the billing team", "REST AP", "", "go for billing"} {
		if g.has(q) {
			t.Errorf("unexpected match for %q", q)
		}
	}
}

func TestSchemaIsStrict(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal(llm.SchemaFor[Documents](), &m); err != nil {
		t.Fatal(err)
	}
	if m["additionalProperties"] != false {
		t.Fatalf("additionalProperties = %v", m["additionalProperties"])
	}
	if _, ok := m["$schema"]; ok {
		t.Fatal("$schema should be stripped")
	}
	if strings.Contains(string(llm.SchemaFor[Documents]()), "$ref") {
		t.Fatal("schema must be fully inlined")
	}
}

// The prompt tells the model which layout and placement values exist: they must
// match the schema, or the model is asked for values the renderer ignores.
func TestPromptMatchesSchemaEnums(t *testing.T) {
	for _, v := range []string{LayoutInline, LayoutLines, LayoutBullets, PlaceBeforeExperience, PlaceAfterEducation} {
		if !strings.Contains(tailorPrompt, v) {
			t.Errorf("tailor prompt does not mention %q", v)
		}
		if !strings.Contains(string(llm.SchemaFor[Documents]()), v) {
			t.Errorf("schema does not contain %q", v)
		}
	}
}

func TestPromptsCoverKeyBehaviours(t *testing.T) {
	for _, want := range []string{"visibly written for this job", "contribute", "Tested", "validated"} {
		if !strings.Contains(tailorPrompt, want) {
			t.Errorf("tailor prompt lost %q", want)
		}
	}
	for _, want := range []string{"validated", "interest", "supported"} {
		if !strings.Contains(verifyPrompt, want) {
			t.Errorf("verify prompt lost %q", want)
		}
	}
}

func TestRefersToOtherEntry(t *testing.T) {
	for text, want := range map[string]bool{
		"Also taught entirely in English":               true,
		"Same as above":                                 true,
		"Studied both degrees entirely in English":      true,
		"Likewise studied in English":                   true,
		"Relevant coursework: Computer Graphics, Image": false,
		"Taught entirely in English":                    false,
		"Graduated in the top 25% of the class":         false,
	} {
		if got := refersToOtherEntry(text); got != want {
			t.Errorf("refersToOtherEntry(%q) = %v, want %v", text, got, want)
		}
	}
	if !isEducationDetail("cv.education[1].details[0]") || isEducationDetail("cv.experience[0].bullets[0]") || isEducationDetail("cv.education[1]") {
		t.Error("isEducationDetail misclassifies ids")
	}
}

// A degree line that points at the other degree is sent back for repair even
// though the model reviewer waves it through.
func TestCrossReferenceTriggersRepair(t *testing.T) {
	bad := baseDocs()
	bad.CV.Education = []Education{{
		Degree: "Bachelor's Degree in Audiovisual Systems Engineering", Institution: "UPF", Period: "2020 - 2025",
		Sources: []string{"Backend developer, Initech (2020-2023)"},
		Details: []Claim{{Text: "Also taught entirely in English", Sources: []string{"Go, PostgreSQL, Docker"}}},
	}}
	fixed := baseDocs()
	fixed.CV.Education = []Education{{
		Degree: "Bachelor's Degree in Audiovisual Systems Engineering", Institution: "UPF", Period: "2020 - 2025",
		Sources: []string{"Backend developer, Initech (2020-2023)"},
		Details: []Claim{{Text: "Relevant coursework: Databases", Sources: []string{"Go, PostgreSQL, Docker"}}},
	}}
	fake := llm.NewFake(
		llm.Reply(analysis),
		llm.Reply(bad), allSupported(bad),
		llm.Reply(fixed), allSupported(fixed),
	)
	res, err := Run(context.Background(), fake, testProfile, testOffer, Options{MaxRepairs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dropped) != 0 {
		t.Fatalf("dropped = %+v", res.Dropped)
	}
	if !strings.Contains(fake.Calls[3].User, "stand on its own") {
		t.Fatalf("repair prompt lacks the cross-reference problem:\n%s", fake.Calls[3].User)
	}
	if got := res.Docs.CV.Education[0].Details[0].Text; !strings.HasPrefix(got, "Relevant coursework") {
		t.Fatalf("detail = %q", got)
	}
}

func TestRankingMustBeInProfile(t *testing.T) {
	profile := testProfile + "\n- Top 25%\n"
	docs := baseDocs()
	docs.CV.Education = []Education{
		{Degree: "A", Institution: "UPF", Ranking: "Top 25%", Sources: []string{"Top 25%"}},
		{Degree: "B", Institution: "UPF", Ranking: "Top 3%", Sources: []string{"Top 25%"}},
	}
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), allSupported(docs))
	res, err := Run(context.Background(), fake, profile, testOffer, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Docs.CV.Education[0].Ranking != "Top 25%" {
		t.Errorf("genuine ranking lost: %q", res.Docs.CV.Education[0].Ranking)
	}
	if res.Docs.CV.Education[1].Ranking != "" {
		t.Errorf("invented ranking kept: %q", res.Docs.CV.Education[1].Ranking)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "Top 3%") {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning about the invented ranking: %v", res.Warnings)
	}
}

func TestEducationHeaderClaimIncludesRanking(t *testing.T) {
	docs := baseDocs()
	docs.CV.Education = []Education{{Degree: "A", Institution: "UPF", Period: "2020 - 2025", Ranking: "Top 25%"}}
	for _, c := range docs.claims() {
		if c.ID == "cv.education[0]" && !strings.Contains(c.Text, "Top 25%") {
			t.Fatalf("header claim must carry the ranking so it is verified: %q", c.Text)
		}
	}
}
