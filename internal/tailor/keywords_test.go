package tailor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/BernatRubio/best-candidate/internal/llm"
)

func kwDocs(texts ...string) *Documents {
	d := &Documents{}
	for _, t := range texts {
		d.CV.Skills = append(d.CV.Skills, Claim{Text: t})
	}
	return d
}

func TestKeywordStatus(t *testing.T) {
	g := newGrounder("Go, PostgreSQL, CI pipeline, Docker")
	a := JobAnalysis{Keywords: []KeyTerm{
		{Term: "Go"},
		{Term: "Continuous Integration", Variants: []string{"CI"}},
		{Term: "PostgreSQL"},
		{Term: "Kafka"},
	}}
	support := []KeywordSupport{
		{Term: "Go", Supported: true, Evidence: "Go"},
		{Term: "Continuous Integration", Supported: true, Evidence: "CI pipeline"},
		{Term: "PostgreSQL", Supported: true, Evidence: "PostgreSQL"},
		{Term: "Kafka", Supported: true, Evidence: "Kafka brokers"}, // quote not in profile
	}
	// "Google" must not match "Go"; the acronym variant matches the full term.
	ats, gaps := keywordStatus(g, kwDocs("Tools: Google Cloud, CI"), a, support, " go postgresql docker ")
	if strings.Join(ats.Present, ",") != "Continuous Integration" {
		t.Errorf("present = %v", ats.Present)
	}
	if strings.Join(ats.Missing, ",") != "Go,PostgreSQL" || len(gaps) != 2 {
		t.Errorf("missing = %v gaps = %v", ats.Missing, gaps)
	}
	if strings.Join(ats.NotInProfile, ",") != "Kafka" {
		t.Errorf("not in profile = %v", ats.NotInProfile)
	}
	// Without any verdict nothing is reported.
	ats, gaps = keywordStatus(g, kwDocs("x"), a, nil, "")
	if len(gaps) != 0 || len(ats.Missing)+len(ats.NotInProfile)+len(ats.Present) != 0 {
		t.Errorf("no verdict should give no report: %+v", ats)
	}
}

func TestMissingKeywordTriggersRepairWithoutDrops(t *testing.T) {
	a := analysis
	a.Keywords = []KeyTerm{{Term: "PostgreSQL", Importance: ImportanceRequired}, {Term: "Kafka"}}
	docs := baseDocs() // contains Go but not PostgreSQL
	repaired := baseDocs()
	repaired.CV.Sections[0].Items[0].Text = "Go, PostgreSQL"

	verify := func(d Documents) func(llm.Request) (json.RawMessage, error) {
		return func(llm.Request) (json.RawMessage, error) {
			var r VerificationReport
			for _, c := range d.claims() {
				r.Verdicts = append(r.Verdicts, Verdict{ID: c.ID, Status: "supported", Reason: "ok"})
			}
			r.Keywords = []KeywordSupport{
				{Term: "PostgreSQL", Supported: true, Evidence: "Go, PostgreSQL, Docker"},
				{Term: "Kafka"}, // not supported: must never cause a repair
			}
			return json.Marshal(r)
		}
	}
	fake := llm.NewFake(llm.Reply(a), llm.Reply(docs), verify(docs), llm.Reply(repaired), verify(repaired))
	res, err := Run(context.Background(), fake, testProfile, testOffer, Options{MaxRepairs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 5 {
		t.Fatalf("want 5 calls, got %d", len(fake.Calls))
	}
	if !strings.Contains(fake.Calls[3].User, "<missing_keywords>") || !strings.Contains(fake.Calls[3].User, "PostgreSQL") {
		t.Fatalf("repair call lacks the missing keyword:\n%s", fake.Calls[3].User)
	}
	if strings.Contains(fake.Calls[3].User, "Kafka") && strings.Contains(fake.Calls[3].User, "missing_keywords>\n- Kafka") {
		t.Fatal("unsupported keyword must not be requested")
	}
	if len(res.Dropped) != 0 {
		t.Fatalf("nothing should be dropped: %+v", res.Dropped)
	}
	if strings.Join(res.ATS.Present, ",") != "PostgreSQL" || strings.Join(res.ATS.NotInProfile, ",") != "Kafka" || len(res.ATS.Missing) != 0 {
		t.Fatalf("ats = %+v", res.ATS)
	}
}

func TestKeywordGapAfterLastRepairIsOnlyReported(t *testing.T) {
	a := analysis
	a.Keywords = []KeyTerm{{Term: "PostgreSQL"}}
	docs := baseDocs()
	verify := func(llm.Request) (json.RawMessage, error) {
		var r VerificationReport
		for _, c := range docs.claims() {
			r.Verdicts = append(r.Verdicts, Verdict{ID: c.ID, Status: "supported"})
		}
		r.Keywords = []KeywordSupport{{Term: "PostgreSQL", Supported: true, Evidence: "Go, PostgreSQL, Docker"}}
		return json.Marshal(r)
	}
	fake := llm.NewFake(llm.Reply(a), llm.Reply(docs), verify)
	res, err := Run(context.Background(), fake, testProfile, testOffer, Options{MaxRepairs: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dropped) != 0 || strings.Join(res.ATS.Missing, ",") != "PostgreSQL" {
		t.Fatalf("dropped=%v ats=%+v", res.Dropped, res.ATS)
	}
}

func TestATSPromptsAndSchema(t *testing.T) {
	for _, want := range []string{"exact form", "missing_keywords", "Skills lines", "No stuffing", "Writing the bullets", "word for word"} {
		if !strings.Contains(tailorPrompt, want) {
			t.Errorf("tailor prompt lost %q", want)
		}
	}
	for _, want := range []string{"variants", "importance", "term"} {
		if !strings.Contains(analyzePrompt, want) {
			t.Errorf("analyze prompt lost %q", want)
		}
	}
	if !strings.Contains(verifyPrompt, "<keywords>") {
		t.Error("verify prompt does not describe the keywords block")
	}
	schema := string(llm.SchemaFor[JobAnalysis]())
	for _, v := range []string{"variants", "importance", "required", "preferred", "mentioned"} {
		if !strings.Contains(schema, v) {
			t.Errorf("analysis schema lacks %q", v)
		}
	}
}

// A keyword that is only a skill in the CV, while the profile shows it used in
// a job, is a gap: the bullet that shows it should name it. A keyword whose
// only evidence is the profile's Skills list is left alone.
func TestKeywordOnlyInSkillsIsAGapWhenProfileShowsUse(t *testing.T) {
	g := newGrounder(testProfile)
	skills := " " + normalize("Go, PostgreSQL, Docker") + " "
	docs := baseDocs()
	docs.CV.Experience[0].Bullets[0].Text = "Built billing services in Go"
	docs.CV.Sections[0].Items = []Claim{{Text: "REST APIs, PostgreSQL"}}
	a := JobAnalysis{Keywords: []KeyTerm{{Term: "REST APIs"}, {Term: "PostgreSQL"}}}
	support := []KeywordSupport{
		{Term: "REST APIs", Supported: true, Evidence: "Built REST APIs in Go for the billing team"},
		{Term: "PostgreSQL", Supported: true, Evidence: "Go, PostgreSQL, Docker"},
	}
	ats, gaps := keywordStatus(g, &docs, a, support, skills)
	if strings.Join(ats.NotInBullets, ",") != "REST APIs" || !gaps["REST APIs"].InCV {
		t.Fatalf("not in bullets = %v gaps = %v", ats.NotInBullets, gaps)
	}
	if _, ok := gaps["PostgreSQL"]; ok {
		t.Fatal("a skill the profile only lists must not trigger a repair")
	}
	if got := missingKeywordsList(a, gaps); !strings.Contains(got, "in no bullet") {
		t.Fatalf("repair note = %q", got)
	}
}

func TestCountBulletsFindsUnchangedLines(t *testing.T) {
	docs := baseDocs()
	docs.CV.Experience[0].Bullets = append(docs.CV.Experience[0].Bullets,
		Claim{Text: "Wrote REST APIs in Go", Sources: []string{"Built REST APIs in Go for the billing team"}})
	total, unchanged := countBullets(&docs.CV)
	if total != 2 || unchanged != 1 {
		t.Fatalf("total=%d unchanged=%d", total, unchanged)
	}
}
