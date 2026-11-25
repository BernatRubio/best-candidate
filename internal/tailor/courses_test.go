package tailor

import (
	"context"
	"strings"
	"testing"

	bestcandidate "github.com/BernatRubio/best-candidate"
	"github.com/BernatRubio/best-candidate/internal/llm"
	"github.com/BernatRubio/best-candidate/internal/profile"
)

const eduProfile = testProfile + `
## Education

### Bachelor's Degree in Computer Science Engineering, Northfield (2020 - 2025)

- Programming and software engineering: Introduction to Programming, Object-Oriented Programming, Data Structures and Algorithms I and II, Software Engineering
- Graphics: Computer Graphics, Advanced Computer Graphics
- Systems: Operating Systems, Networks, Introduction to Parallel and Distributed Programming (OpenMP, OpenMPI, CUDA)

### Bachelor's Degree in Media Engineering, Northfield (2020 - 2025)

- Image and video: Image Processing, Video Processing, Computer Graphics, Advanced Computer Graphics
- Programming: Introduction to Programming, Object-Oriented Programming, Data Structures and Algorithms I and II
- Highest grades (9.0 or above): Wireless Sensor Networks 9.7, Operating Systems 9.3, Bachelor's thesis 9.3

## Languages

- Spanish: native
`

func claims(texts ...string) []Claim {
	var out []Claim
	for _, t := range texts {
		out = append(out, Claim{Text: t, Sources: []string{"Computer Graphics, Advanced Computer Graphics"}})
	}
	return out
}

func eduDocs(cs, av []Claim) *Documents {
	d := baseDocs()
	d.CV.Education = []Education{
		{Degree: "Bachelor's Degree in Computer Science Engineering", Institution: "Northfield", Period: "2020 - 2025",
			Sources: []string{"Bachelor's Degree in Computer Science Engineering, Northfield (2020 - 2025)"}, Details: cs},
		{Degree: "Bachelor's Degree in Media Engineering", Institution: "Northfield", Period: "2020 - 2025",
			Sources: []string{"Bachelor's Degree in Media Engineering, Northfield (2020 - 2025)"}, Details: av},
	}
	return &d
}

func TestCourseCatalog(t *testing.T) {
	cat := profile.Parse(eduProfile).Courses()
	index := map[string]int{}
	for i, c := range cat {
		index[c] = i
	}
	for _, want := range []string{"computer graphics", "advanced computer graphics", "operating systems",
		"introduction to parallel and distributed programming", "data structures and algorithms i and ii"} {
		if _, ok := index[want]; !ok {
			t.Errorf("catalog lacks %q: %v", want, cat)
		}
	}
	for _, bad := range []string{"python", "spanish", "wireless sensor networks 9 7"} {
		if _, ok := index[bad]; ok {
			t.Errorf("catalog must not contain %q (outside Education or a grade line)", bad)
		}
	}
	if index["advanced computer graphics"] > index["computer graphics"] {
		t.Error("longer names must come first so they are masked before their substrings")
	}
	if len(profile.Parse("# Nobody\n\n## Skills\n\n- Go: a, b, c\n").Courses()) != 0 {
		t.Error("no Education section must give an empty catalog")
	}
}

func TestMentionsDoNotReportSubstrings(t *testing.T) {
	cat := profile.Parse(eduProfile).Courses()
	got := courseMentions("Relevant coursework: Advanced Computer Graphics, Image Processing.", cat)
	if strings.Join(got, "|") != "advanced computer graphics|image processing" {
		t.Fatalf("got %v", got)
	}
}

func TestSharedCourseAcrossDegreesIsFlagged(t *testing.T) {
	cat := profile.Parse(eduProfile).Courses()
	docs := eduDocs(
		claims("Relevant coursework: Object-Oriented Programming, Computer Graphics, Operating Systems."),
		claims("Relevant coursework: Image Processing, Computer Graphics, Video Processing.", "Top grade in Operating Systems."),
	)
	issues := duplicateCourseIssues(docs, cat)
	if len(issues) != 2 {
		t.Fatalf("issues = %v", issues)
	}
	for _, id := range []string{"cv.education[1].details[0]", "cv.education[1].details[1]"} {
		if r := issues[id]; !strings.Contains(r, "only one degree") {
			t.Errorf("%s: %q", id, r)
		}
	}
	if !strings.Contains(issues["cv.education[1].details[0]"], "computer graphics") ||
		!strings.Contains(issues["cv.education[1].details[1]"], "operating systems") {
		t.Errorf("reasons should name the course: %v", issues)
	}
}

func TestDistinctCoursesAreNotFlagged(t *testing.T) {
	cat := profile.Parse(eduProfile).Courses()
	// "Computer Graphics" is a substring of "Advanced Computer Graphics" but they are different courses.
	docs := eduDocs(
		claims("Relevant coursework: Advanced Computer Graphics, Operating Systems."),
		claims("Relevant coursework: Computer Graphics, Image Processing."),
	)
	if issues := duplicateCourseIssues(docs, cat); len(issues) != 0 {
		t.Fatalf("false positive: %v", issues)
	}
}

func TestDuplicateDetectedWithoutCatalog(t *testing.T) {
	// A profile with no recognisable course lists: coursework lines are still compared.
	docs := eduDocs(
		claims("Relevant coursework: Thermodynamics, Fluid Mechanics, and Control Theory."),
		claims("Relevant coursework: Control Theory, Statics."),
	)
	issues := duplicateCourseIssues(docs, nil)
	if len(issues) != 1 || !strings.Contains(issues["cv.education[1].details[0]"], "control theory") {
		t.Fatalf("issues = %v", issues)
	}
}

func TestDedupeCourseworkSafetyNet(t *testing.T) {
	cat := profile.Parse(eduProfile).Courses()
	docs := eduDocs(
		claims("Relevant coursework: Object-Oriented Programming, Computer Graphics, Operating Systems."),
		claims("Relevant coursework: Image Processing, Computer Graphics, Video Processing.", "Top grade in Operating Systems.", "Taught entirely in English."),
	)
	warnings := dedupeCoursework(docs, cat)
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v", warnings)
	}
	cs, av := docs.CV.Education[0], docs.CV.Education[1]
	if len(cs.Details) != 1 || !strings.Contains(cs.Details[0].Text, "Computer Graphics") {
		t.Fatalf("first degree must keep its courses: %+v", cs.Details)
	}
	if len(av.Details) != 2 {
		t.Fatalf("the non-list line naming a repeated course must go, the others stay: %+v", av.Details)
	}
	if got := av.Details[0].Text; got != "Relevant coursework: Image Processing, Video Processing." {
		t.Fatalf("list rebuilt wrongly: %q", got)
	}
	if av.Details[1].Text != "Taught entirely in English." {
		t.Fatalf("unrelated line changed: %q", av.Details[1].Text)
	}
	if issues := duplicateCourseIssues(docs, cat); len(issues) != 0 {
		t.Fatalf("still duplicated after dedupe: %v", issues)
	}
}

func TestDedupeDropsLineThatBecomesEmpty(t *testing.T) {
	cat := profile.Parse(eduProfile).Courses()
	docs := eduDocs(
		claims("Relevant coursework: Computer Graphics, Operating Systems."),
		claims("Relevant coursework: Computer Graphics, Operating Systems."),
	)
	dedupeCoursework(docs, cat)
	if len(docs.CV.Education[1].Details) != 0 {
		t.Fatalf("an emptied line must be removed: %+v", docs.CV.Education[1].Details)
	}
}

// The model repeats a course across degrees; the repair round fixes it, so
// nothing is removed and no warning is raised.
func TestSharedCourseTriggersRepair(t *testing.T) {
	bad := eduDocs(
		claims("Relevant coursework: Object-Oriented Programming, Computer Graphics."),
		claims("Relevant coursework: Image Processing, Computer Graphics."),
	)
	fixed := eduDocs(
		claims("Relevant coursework: Object-Oriented Programming, Computer Graphics."),
		claims("Relevant coursework: Image Processing, Video Processing."),
	)
	fake := llm.NewFake(
		llm.Reply(analysis),
		llm.Reply(bad), allSupported(*bad),
		llm.Reply(fixed), allSupported(*fixed),
	)
	res, err := Run(context.Background(), fake, profile.Parse(eduProfile), testOffer, Options{MaxRepairs: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dropped) != 0 {
		t.Fatalf("dropped = %+v", res.Dropped)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "course") {
			t.Fatalf("no course warning expected: %v", res.Warnings)
		}
	}
	if !strings.Contains(fake.Calls[3].User, "only one degree") {
		t.Fatalf("repair prompt must explain the duplicate:\n%s", fake.Calls[3].User)
	}
	if got := res.Docs.CV.Education[1].Details[0].Text; strings.Contains(got, "Computer Graphics") {
		t.Fatalf("second degree still lists the shared course: %q", got)
	}
}

// If the model keeps the duplicate through every repair round, the code removes
// it from the later degree rather than shipping it.
func TestPersistentSharedCourseIsDeduplicated(t *testing.T) {
	bad := eduDocs(
		claims("Relevant coursework: Object-Oriented Programming, Computer Graphics."),
		claims("Relevant coursework: Image Processing, Computer Graphics."),
	)
	fake := llm.NewFake(
		llm.Reply(analysis),
		llm.Reply(bad), allSupported(*bad),
		llm.Reply(bad), allSupported(*bad),
	)
	res, err := Run(context.Background(), fake, profile.Parse(eduProfile), testOffer, Options{MaxRepairs: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Docs.CV.Education[1].Details[0].Text; got != "Relevant coursework: Image Processing." {
		t.Fatalf("second degree = %q", got)
	}
	if got := res.Docs.CV.Education[0].Details[0].Text; !strings.Contains(got, "Computer Graphics") {
		t.Fatalf("first degree must keep the course: %q", got)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "computer graphics") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a warning about the removed course: %v", res.Warnings)
	}
	if len(res.Dropped) != 0 {
		t.Fatalf("a repeated course is not a false claim; nothing should be dropped: %+v", res.Dropped)
	}
}

// The profile template documents the course-list format; keep its example
// parseable so the documentation cannot drift from the code.
func TestExampleProfileCourseListsAreParsed(t *testing.T) {
	cat := profile.Parse(bestcandidate.ExampleProfile).Courses()
	have := map[string]bool{}
	for _, c := range cat {
		have[c] = true
	}
	for _, want := range []string{"algorithms", "operating systems", "linear algebra", "statistics"} {
		if !have[want] {
			t.Errorf("template course %q is not in the catalog: %v", want, cat)
		}
	}
	for _, grade := range []string{"databases 9 4", "statistics 9 1"} {
		if have[grade] {
			t.Errorf("a course written with a grade must not be a catalog entry: %q", grade)
		}
	}
}
