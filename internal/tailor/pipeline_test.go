package tailor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/BernatRubio/best-candidate/internal/llm"
	"github.com/BernatRubio/best-candidate/internal/profile"
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
	MustHave: []string{"Go", "Kubernetes"}, Keywords: []KeyTerm{{Term: "Go", Importance: ImportanceRequired}}, Tone: "technical",
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
	verify := func(r llm.Request) (json.RawMessage, error) {
		raw, _ := allSupported(docs)(r)
		var rep VerificationReport
		_ = json.Unmarshal(raw, &rep)
		rep.Coverage = []Coverage{
			{Requirement: "Go", Covered: true, Evidence: "Built REST APIs in Go"},
			{Requirement: "Kubernetes", Covered: true, Evidence: "Ran Kubernetes clusters"},
		}
		return json.Marshal(rep)
	}
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), verify, verify)

	res, err := Run(context.Background(), fake, profile.Parse(testProfile), testOffer, Options{MaxRepairs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fake.Calls[1].Schema), `"coverage"`) {
		t.Fatal("the writer must not be asked for coverage")
	}
	if len(fake.Calls) != 4 {
		t.Fatalf("want 4 model calls, got %d", len(fake.Calls))
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
		llm.Reply(docs), allSupported(docs), allSupported(docs), // round 1: reviewer is fooled, quote check is not
		llm.Reply(docs), allSupported(docs), allSupported(docs), // repair returns the same lie
	)

	res, err := Run(context.Background(), fake, profile.Parse(testProfile), testOffer, Options{MaxRepairs: 1})
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
	repair := fake.Calls[4].User
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
		llm.Reply(bad), flag, flag,
		llm.Reply(fixed), allSupported(fixed), allSupported(fixed),
	)
	res, err := Run(context.Background(), fake, profile.Parse(testProfile), testOffer, Options{MaxRepairs: 2})
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
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), allSupported(docs), allSupported(docs))
	res, err := Run(context.Background(), fake, profile.Parse(testProfile), testOffer, Options{Lang: "es"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Lang != "es" || !strings.Contains(fake.Calls[1].User, "<language>\nes\n</language>") {
		t.Fatalf("lang = %q", res.Lang)
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

// The prompt names the values the model should use for extra sections, and the
// dedicated fields it must fill; they must exist in the schema, or the model is
// asked for things the renderer ignores.
func TestPromptMatchesSchema(t *testing.T) {
	schema := string(llm.SchemaFor[Documents]())
	for _, v := range []string{LayoutBullets, PlaceAfterEducation, "projects", "projects_first", "skills", "languages", "link", "relevance"} {
		if !strings.Contains(tailorPrompt, v) {
			t.Errorf("tailor prompt does not mention %q", v)
		}
		if !strings.Contains(schema, v) {
			t.Errorf("schema does not contain %q", v)
		}
	}
	// The renderer still understands every layout and placement.
	for _, v := range []string{LayoutInline, LayoutLines, LayoutBullets, PlaceBeforeExperience, PlaceAfterEducation} {
		if !strings.Contains(schema, v) {
			t.Errorf("schema does not contain %q", v)
		}
	}
}

func TestPromptsCoverKeyBehaviours(t *testing.T) {
	for _, want := range []string{"visibly written for this job", "contribute", "tested", "validated"} {
		if !strings.Contains(tailorPrompt, want) {
			t.Errorf("tailor prompt lost %q", want)
		}
	}
	for _, want := range []string{"validated", "supported", "<requirements>", "<keywords>"} {
		if !strings.Contains(verifyCVPrompt, want) {
			t.Errorf("CV verify prompt lost %q", want)
		}
	}
	for _, want := range []string{"validated", "interest", "supported", "no `cited` quotes"} {
		if !strings.Contains(verifyLetterPrompt, want) {
			t.Errorf("letter verify prompt lost %q", want)
		}
	}
	if strings.Contains(verifyCVPrompt, "interest in the role") || strings.Contains(verifyLetterPrompt, "<keywords>") {
		t.Error("each verify prompt must hold only its own part")
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
	if (ClaimRef{kindEducationDetail, 1, 0}).String() != "cv.education[1].details[0]" || (ClaimRef{kindEducation, 1, 0}).String() != "cv.education[1]" {
		t.Error("ClaimRef renders the wrong id")
	}
}

// A degree line that points at the other degree is sent back for repair even
// though the model reviewer waves it through.
func TestCrossReferenceTriggersRepair(t *testing.T) {
	bad := baseDocs()
	bad.CV.Education = []Education{{
		Degree: "Bachelor's Degree in Media Engineering", Institution: "Northfield", Period: "2020 - 2025",
		Sources: []string{"Backend developer, Initech (2020-2023)"},
		Details: []Claim{{Text: "Also taught entirely in English", Sources: []string{"Go, PostgreSQL, Docker"}}},
	}}
	fixed := baseDocs()
	fixed.CV.Education = []Education{{
		Degree: "Bachelor's Degree in Media Engineering", Institution: "Northfield", Period: "2020 - 2025",
		Sources: []string{"Backend developer, Initech (2020-2023)"},
		Details: []Claim{{Text: "Relevant coursework: Databases", Sources: []string{"Go, PostgreSQL, Docker"}}},
	}}
	fake := llm.NewFake(
		llm.Reply(analysis),
		llm.Reply(bad), allSupported(bad), allSupported(bad),
		llm.Reply(fixed), allSupported(fixed), allSupported(fixed),
	)
	res, err := Run(context.Background(), fake, profile.Parse(testProfile), testOffer, Options{MaxRepairs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dropped) != 0 {
		t.Fatalf("dropped = %+v", res.Dropped)
	}
	if !strings.Contains(fake.Calls[4].User, "stand on its own") {
		t.Fatalf("repair prompt lacks the cross-reference problem:\n%s", fake.Calls[4].User)
	}
	if got := res.Docs.CV.Education[0].Details[0].Text; !strings.HasPrefix(got, "Relevant coursework") {
		t.Fatalf("detail = %q", got)
	}
}

func TestRankingMustBeInProfile(t *testing.T) {
	text := testProfile + "\n- Top 25%\n"
	docs := baseDocs()
	docs.CV.Education = []Education{
		{Degree: "A", Institution: "Northfield", Ranking: "Top 25%", Sources: []string{"Top 25%"}},
		{Degree: "B", Institution: "Northfield", Ranking: "Top 3%", Sources: []string{"Top 25%"}},
	}
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), allSupported(docs), allSupported(docs))
	res, err := Run(context.Background(), fake, profile.Parse(text), testOffer, Options{})
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
	docs.CV.Education = []Education{{Degree: "A", Institution: "Northfield", Period: "2020 - 2025", Ranking: "Top 25%"}}
	for _, c := range docs.claims() {
		if c.ID == "cv.education[0]" && !strings.Contains(c.Text, "Top 25%") {
			t.Fatalf("header claim must carry the ranking so it is verified: %q", c.Text)
		}
	}
}

const fillProfile = `# Ada Example
ada@example.com
https://github.com/ada/orion

## Skills

- Programming: Go, SQL
- Tools: Git, Docker
- Used at Initech: Postgres, Kafka

## Languages

- Spanish: native
- English: B2 (Cambridge)
- Studied both degrees entirely in English
`

func minimalDocs() Documents {
	return Documents{
		CV:     CV{Name: "Ada Example", Contact: []string{"ada@example.com"}},
		Letter: Letter{Greeting: "Dear team,", Closing: "Regards,", Signature: "Ada Example"},
	}
}

// A model that leaves skills and languages empty still gets them, taken from
// the profile with their exact lines as evidence.
func TestEmptySkillsAndLanguagesAreFilledFromProfile(t *testing.T) {
	docs := minimalDocs()
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), allSupported(docs))
	res, err := Run(context.Background(), fake, profile.Parse(fillProfile), testOffer, Options{})
	if err != nil {
		t.Fatal(err)
	}
	skills, langs := res.Docs.CV.Skills, res.Docs.CV.Languages
	if len(skills) != 3 || skills[0].Text != "Programming: Go, SQL" || skills[0].Sources[0] != "Programming: Go, SQL" {
		t.Fatalf("skills = %+v", skills)
	}
	if len(langs) != 2 || langs[0].Text != "Spanish (native)" || langs[1].Text != "English (B2, Cambridge)" {
		t.Fatalf("languages = %+v (the remark line must be skipped)", langs)
	}
	joined := strings.Join(res.Warnings, " | ")
	if !strings.Contains(joined, "Skills block") || !strings.Contains(joined, "Languages block") {
		t.Fatalf("warnings = %v", res.Warnings)
	}
}

// The model's own skills and languages are never overwritten.
func TestModelSkillsAreKept(t *testing.T) {
	docs := minimalDocs()
	docs.CV.Skills = []Claim{{Text: "Programming: Go", Sources: []string{"Programming: Go, SQL"}}}
	docs.CV.Languages = []Claim{{Text: "English (B2, Cambridge)", Sources: []string{"English: B2 (Cambridge)"}}}
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), allSupported(docs))
	res, err := Run(context.Background(), fake, profile.Parse(fillProfile), testOffer, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Docs.CV.Skills) != 1 || len(res.Docs.CV.Languages) != 1 {
		t.Fatalf("model output replaced: %+v %+v", res.Docs.CV.Skills, res.Docs.CV.Languages)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "block") {
			t.Fatalf("no fill-in warning expected: %v", w)
		}
	}
}

func TestProjectLinkMustBeInProfile(t *testing.T) {
	docs := minimalDocs()
	docs.CV.Projects = []Project{
		{Name: "Orion", Link: "https://github.com/ada/orion", Sources: []string{"https://github.com/ada/orion"}},
		{Name: "Other", Link: "https://made-up.example/x", Sources: []string{"https://github.com/ada/orion"}},
	}
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), allSupported(docs))
	res, err := Run(context.Background(), fake, profile.Parse(fillProfile), testOffer, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Docs.CV.Projects[0].Link == "" || res.Docs.CV.Projects[1].Link != "" {
		t.Fatalf("links = %q / %q", res.Docs.CV.Projects[0].Link, res.Docs.CV.Projects[1].Link)
	}
	if !strings.Contains(strings.Join(res.Warnings, " "), "made-up.example") {
		t.Fatalf("no warning for the invented link: %v", res.Warnings)
	}
}

// The fact-checker assesses the requirements, and its evidence quotes are
// still checked against the profile.
func TestVerifierCoverageIsUsed(t *testing.T) {
	docs := minimalDocs()
	verify := func(req llm.Request) (json.RawMessage, error) {
		if !strings.Contains(req.User, "must have: Go") || !strings.Contains(req.User, "must have: Kubernetes") {
			t.Errorf("verifier was not given the requirements:\n%s", req.User)
		}
		return json.Marshal(VerificationReport{Coverage: []Coverage{
			{Requirement: "Go", Covered: true, Evidence: "Programming: Go, SQL"},
			{Requirement: "Kubernetes", Covered: true, Evidence: "Ran Kubernetes clusters"},
		}})
	}
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), verify)
	res, err := Run(context.Background(), fake, profile.Parse(fillProfile), testOffer, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Docs.Coverage) != 2 {
		t.Fatalf("coverage = %+v", res.Docs.Coverage)
	}
	for _, c := range res.Docs.Coverage {
		switch c.Requirement {
		case "Go":
			if !c.Covered {
				t.Error("Go should be covered")
			}
		case "Kubernetes":
			if c.Covered {
				t.Error("an invented evidence quote must downgrade Kubernetes")
			}
		}
	}
}

func TestSchemaHasDedicatedBlocks(t *testing.T) {
	schema := string(llm.SchemaFor[Documents]())
	for _, key := range []string{`"projects"`, `"skills"`, `"languages"`, `"projects_first"`} {
		if !strings.Contains(schema, key) {
			t.Errorf("schema lacks %s", key)
		}
	}
}

func TestCleanEducationTitles(t *testing.T) {
	docs := minimalDocs()
	docs.CV.Education = []Education{
		{Degree: "Bachelor's Degree in Computer Science Engineering, Northfield (2020 - 2025)", Institution: "Northfield School of Engineering", Period: "2020 - 2025"},
		{Degree: "Master's Degree in Mobile Programming (2026 - Present)", Institution: "Example Institute", Period: "2026 - Present"},
		{Degree: "Bachelor's Degree in Media Engineering", Institution: "Northfield", Period: "2020 - 2025"},
		{Degree: "Degree in Physics, Optics (Advanced)", Institution: "Northfield", Period: "2019 - 2023"}, // not an institution: untouched
	}
	cleanEducationTitles(&docs)
	want := []string{
		"Bachelor's Degree in Computer Science Engineering",
		"Master's Degree in Mobile Programming",
		"Bachelor's Degree in Media Engineering",
		"Degree in Physics, Optics (Advanced)",
	}
	for i, w := range want {
		if got := docs.CV.Education[i].Degree; got != w {
			t.Errorf("degree %d = %q, want %q", i, got, w)
		}
	}
}

func TestCleanRecipient(t *testing.T) {
	cases := []struct{ in, company, want string }{
		{"hiring_team", "Acme", "Acme"},
		{"hiring_team", "", ""},
		{"HR_Department", "Acme", "Acme"},
		{"Acme Hiring Team", "Acme", "Acme Hiring Team"},
		{"Acme", "Acme", "Acme"},
		{"", "Acme", ""},
	}
	for _, c := range cases {
		docs := minimalDocs()
		docs.Letter.Recipient = c.in
		cleanRecipient(&docs, JobAnalysis{Company: c.company})
		if docs.Letter.Recipient != c.want {
			t.Errorf("cleanRecipient(%q, company %q) = %q, want %q", c.in, c.company, docs.Letter.Recipient, c.want)
		}
	}
}

func TestCourseworkLeadRemark(t *testing.T) {
	for text, want := range map[string]bool{
		"Relevant coursework (degree studied entirely in English): Data Structures, Graphics": true,
		"Relevant courses (taught in English): Databases":                                     true,
		"Relevant coursework: Data Structures (Python), Graphics":                             false,
		"Taught entirely in English":                                                          false,
		"Highest grades (9.0 or above): Graphics 9.5":                                         false,
	} {
		if got := courseworkLeadHasRemark(text); got != want {
			t.Errorf("courseworkLeadHasRemark(%q) = %v, want %v", text, got, want)
		}
	}
}

// A coursework line with a remark in its heading is sent back for repair.
func TestCourseworkRemarkTriggersRepair(t *testing.T) {
	bad := baseDocs()
	bad.CV.Education = []Education{{
		Degree: "Bachelor's Degree in Computer Science Engineering", Institution: "Northfield", Period: "2020 - 2025",
		Sources: []string{"Backend developer, Initech (2020-2023)"},
		Details: []Claim{{Text: "Relevant coursework (degree studied entirely in English): Databases", Sources: []string{"Go, PostgreSQL, Docker"}}},
	}}
	fixed := baseDocs()
	fixed.CV.Education = []Education{{
		Degree: "Bachelor's Degree in Computer Science Engineering", Institution: "Northfield", Period: "2020 - 2025",
		Sources: []string{"Backend developer, Initech (2020-2023)"},
		Details: []Claim{
			{Text: "Relevant coursework: Databases", Sources: []string{"Go, PostgreSQL, Docker"}},
			{Text: "Taught entirely in English", Sources: []string{"Go, PostgreSQL, Docker"}},
		},
	}}
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(bad), allSupported(bad), allSupported(bad), llm.Reply(fixed), allSupported(fixed), allSupported(fixed))
	res, err := Run(context.Background(), fake, profile.Parse(testProfile), testOffer, Options{MaxRepairs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fake.Calls[4].User, "own line") {
		t.Fatalf("repair prompt lacks the problem:\n%s", fake.Calls[4].User)
	}
	if got := res.Docs.CV.Education[0].Details; len(got) != 2 || strings.Contains(got[0].Text, "(") {
		t.Fatalf("details = %+v", got)
	}
}

func eduEntry(degree string, relevance int, details ...string) Education {
	e := Education{Degree: degree, Institution: "Northfield", Relevance: relevance}
	for _, d := range details {
		e.Details = append(e.Details, Claim{Text: d})
	}
	return e
}

func degrees(docs *Documents) string {
	var out []string
	for _, e := range docs.CV.Education {
		out = append(out, e.Degree)
	}
	return strings.Join(out, ",")
}

func TestSortEducationByRelevance(t *testing.T) {
	docs := minimalDocs()
	docs.CV.Education = []Education{eduEntry("Media", 3), eduEntry("Physics", 5), eduEntry("Computing", 9), eduEntry("Law", 5)}
	sortEducation(&docs, JobAnalysis{})
	// Highest first; equal scores keep the order the model wrote them in.
	if got := degrees(&docs); got != "Computing,Physics,Law,Media" {
		t.Fatalf("order = %s", got)
	}
}

func TestSortEducationTiesUseOfferKeywords(t *testing.T) {
	docs := minimalDocs()
	docs.CV.Education = []Education{
		eduEntry("Media", 5, "Relevant coursework: Video Processing"),
		eduEntry("Computing", 5, "Relevant coursework: Python, Databases, Algorithms"),
	}
	sortEducation(&docs, JobAnalysis{Keywords: []KeyTerm{{Term: "Python"}, {Term: "Databases"}}, MustHave: []string{"Python"}})
	if got := degrees(&docs); got != "Computing,Media" {
		t.Fatalf("order = %s", got)
	}
	// With no keyword signal either, the model's order is kept.
	docs.CV.Education = []Education{eduEntry("A", 5), eduEntry("B", 5)}
	sortEducation(&docs, JobAnalysis{Keywords: []KeyTerm{{Term: "Python"}}})
	if got := degrees(&docs); got != "A,B" {
		t.Fatalf("order = %s", got)
	}
}

// The model lists the unrelated degree first (the order of the profile); the
// pipeline puts the relevant one first before anything else looks at the
// entries.
func TestRunOrdersDegreesByRelevance(t *testing.T) {
	docs := baseDocs()
	docs.CV.Education = []Education{
		{Relevance: 2, Degree: "Media Engineering", Institution: "Northfield", Period: "2020 - 2025",
			Sources: []string{"Backend developer, Initech (2020-2023)"}},
		{Relevance: 9, Degree: "Computer Science", Institution: "Northfield", Period: "2020 - 2025",
			Sources: []string{"Backend developer, Initech (2020-2023)"}},
	}
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), func(llm.Request) (json.RawMessage, error) {
		// The reviewer sees the claims in the sorted order.
		return allSupported(Documents{CV: CV{Education: []Education{docs.CV.Education[1], docs.CV.Education[0]}}})(llm.Request{})
	}, llm.Reply(VerificationReport{}), llm.Reply(VerificationReport{}), llm.Reply(VerificationReport{}))
	res, err := Run(context.Background(), fake, profile.Parse(testProfile), testOffer, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := degrees(&res.Docs); got != "Computer Science,Media Engineering" {
		t.Fatalf("order = %s", got)
	}
	if !strings.Contains(fake.Calls[2].User, "id: cv.education[0]\ntext: Computer Science") {
		t.Fatalf("the verifier must see the sorted order:\n%s", fake.Calls[2].User)
	}
}

// A reviewer that stops part way through the list is asked again about the
// claims it skipped, and only those, before they count as unreviewed.
func TestSkippedClaimsAreReviewedAgain(t *testing.T) {
	docs := baseDocs()
	claims := docs.claims()
	first := func(llm.Request) (json.RawMessage, error) {
		return json.Marshal(VerificationReport{Verdicts: []Verdict{{ID: claims[0].ID, Status: "supported"}}})
	}
	var second llm.Request
	rest := func(r llm.Request) (json.RawMessage, error) {
		second = r
		var rep VerificationReport
		for _, c := range claims[1:] {
			rep.Verdicts = append(rep.Verdicts, Verdict{ID: c.ID, Status: "supported"})
		}
		return json.Marshal(rep)
	}
	// CV check, its re-ask, then the letter check.
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), first, rest, allSupported(docs))
	res, err := Run(context.Background(), fake, profile.Parse(testProfile), testOffer, Options{MaxRepairs: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dropped) != 0 {
		t.Fatalf("dropped = %+v", res.Dropped)
	}
	if strings.Contains(second.User, "id: "+claims[0].ID+"\n") || !strings.Contains(second.User, "id: "+claims[1].ID+"\n") {
		t.Fatalf("the second review must hold only the skipped claims:\n%s", second.User)
	}
	if strings.Contains(second.User, "<requirements>") {
		t.Fatal("requirements are assessed once, not again")
	}
}

// Claims the fact-checker keeps skipping are not sent to the writer: the same
// draft is verified again, and they are dropped only when the rounds run out.
func TestUnreviewedClaimsAreVerifiedAgainNotRepaired(t *testing.T) {
	docs := baseDocs()
	claims := docs.claims()
	allButLast := func(llm.Request) (json.RawMessage, error) {
		var rep VerificationReport
		for _, c := range claims[:len(claims)-1] {
			rep.Verdicts = append(rep.Verdicts, Verdict{ID: c.ID, Status: "supported"})
		}
		return json.Marshal(rep)
	}
	none := llm.Reply(VerificationReport{})
	// The last claim is the letter paragraph. Each round: the CV check, the
	// letter check and its re-ask; round 2 follows with no tailoring between.
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), allButLast, none, none, allButLast, none, none)
	res, err := Run(context.Background(), fake, profile.Parse(testProfile), testOffer, Options{MaxRepairs: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 8 {
		t.Fatalf("calls = %d, want 8 (no repair call)", len(fake.Calls))
	}
	last := claims[len(claims)-1].ID
	if len(res.Dropped) != 1 || res.Dropped[0].ID != last || res.Dropped[0].Reason != notReviewed {
		t.Fatalf("dropped = %+v", res.Dropped)
	}
}

// A keyword that only the Skills block filled in from the profile contains is
// present in the CV, so the ATS result must be computed after the fill.
func TestATSCountsSkillsFilledFromProfile(t *testing.T) {
	a := analysis
	a.Keywords = []KeyTerm{{Term: "Docker", Importance: ImportanceRequired}}
	verify := func(llm.Request) (json.RawMessage, error) {
		return json.Marshal(VerificationReport{Keywords: []KeywordSupport{{Term: "Docker", Supported: true, Evidence: "Tools: Git, Docker"}}})
	}
	fake := llm.NewFake(llm.Reply(a), llm.Reply(minimalDocs()), verify)
	res, err := Run(context.Background(), fake, profile.Parse(fillProfile), testOffer, Options{MaxRepairs: 0})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.ATS.Present, ",") != "Docker" || len(res.ATS.Missing) != 0 {
		t.Fatalf("ats = %+v", res.ATS)
	}
}

// Content cut after the run (to keep the CV on one page) is reflected once the
// ATS result is refreshed.
func TestRefreshATSAfterCut(t *testing.T) {
	a := analysis
	a.Keywords = []KeyTerm{{Term: "Docker", Importance: ImportanceRequired}}
	verify := func(llm.Request) (json.RawMessage, error) {
		return json.Marshal(VerificationReport{Keywords: []KeywordSupport{{Term: "Docker", Supported: true, Evidence: "Tools: Git, Docker"}}})
	}
	fake := llm.NewFake(llm.Reply(a), llm.Reply(minimalDocs()), verify)
	res, err := Run(context.Background(), fake, profile.Parse(fillProfile), testOffer, Options{MaxRepairs: 0})
	if err != nil {
		t.Fatal(err)
	}
	res.Docs.CV.Skills = nil
	res.RefreshATS()
	if len(res.ATS.Present) != 0 || strings.Join(res.ATS.Missing, ",") != "Docker" {
		t.Fatalf("ats = %+v", res.ATS)
	}
}

// The CV and the letter are fact-checked in separate calls: the CV call holds
// the requirements and keywords, the letter call only the paragraphs, with its
// own prompt and a verdicts-only schema.
func TestCVAndLetterAreCheckedSeparately(t *testing.T) {
	docs := baseDocs()
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), allSupported(docs), allSupported(docs))
	if _, err := Run(context.Background(), fake, profile.Parse(testProfile), testOffer, Options{}); err != nil {
		t.Fatal(err)
	}
	cv, letter := fake.Calls[2], fake.Calls[3]
	if cv.System != verifyCVPrompt || !strings.Contains(cv.User, "<requirements>") || strings.Contains(cv.User, "id: letter.") {
		t.Fatalf("CV call:\n%s", cv.User)
	}
	if letter.System != verifyLetterPrompt || strings.Contains(letter.User, "<requirements>") || strings.Contains(letter.User, "id: cv.") ||
		!strings.Contains(letter.User, "id: letter.paragraphs[0]") || strings.Contains(string(letter.Schema), `"coverage"`) {
		t.Fatalf("letter call:\n%s\nschema: %s", letter.User, letter.Schema)
	}
}

// A letter without paragraphs costs no letter call.
func TestEmptyLetterIsNotChecked(t *testing.T) {
	docs := baseDocs()
	docs.Letter.Paragraphs = nil
	fake := llm.NewFake(llm.Reply(analysis), llm.Reply(docs), allSupported(docs))
	if _, err := Run(context.Background(), fake, profile.Parse(testProfile), testOffer, Options{}); err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 3 {
		t.Fatalf("calls = %d", len(fake.Calls))
	}
}
