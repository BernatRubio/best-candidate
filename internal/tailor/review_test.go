package tailor

import (
	"slices"
	"strings"
	"testing"

	"github.com/BernatRubio/best-candidate/internal/profile"
)

func supportedReport(d *Documents) VerificationReport {
	var r VerificationReport
	for _, c := range d.claims() {
		r.Verdicts = append(r.Verdicts, Verdict{ID: c.ID, Status: "supported"})
	}
	return r
}

func TestReviewFindings(t *testing.T) {
	prof := profile.Parse(testProfile)
	rv := newDraftReview(prof, analysis)

	d := baseDocs()
	if f := rv.Review(&d, supportedReport(&d)); !f.clean() {
		t.Fatalf("an honest draft must be clean: %+v", f)
	}

	// A quote that is not in the profile is flagged even when the reviewer
	// waves the claim through.
	d = baseDocs()
	d.CV.Experience[0].Bullets[0].Sources = []string{"Led a team of 10"}
	f := rv.Review(&d, supportedReport(&d))
	if !strings.Contains(f.Issues["cv.experience[0].bullets[0]"], "not found in the profile") {
		t.Fatalf("issues = %v", f.Issues)
	}

	// A claim the reviewer skipped counts as unreviewed, not as an issue, and
	// is not sent to the writer; a verdict is kept.
	d = baseDocs()
	f = rv.Review(&d, VerificationReport{Verdicts: []Verdict{{ID: "cv.experience[0]", Status: "exaggerated", Reason: "x"}}})
	if f.Issues["cv.experience[0]"] != "exaggerated: x" || f.Issues["cv.experience[0].bullets[0]"] != "" {
		t.Fatalf("issues = %v", f.Issues)
	}
	if !slices.Contains(f.Unreviewed, "cv.experience[0].bullets[0]") || f.onlyUnreviewed() {
		t.Fatalf("unreviewed = %v", f.Unreviewed)
	}
	if p := f.problems(&d); !strings.Contains(p, "cv.experience[0]: ") || strings.Contains(p, "bullets[0]") {
		t.Fatalf("problems = %s", p)
	}

	// Unreviewed claims are dropped when the rounds run out.
	dropped, _ := rv.GiveUp(&d, f)
	var reasons []string
	for _, x := range dropped {
		reasons = append(reasons, x.ID+"="+x.Reason)
	}
	if !slices.Contains(reasons, "cv.experience[0].bullets[0]="+notReviewed) {
		t.Fatalf("dropped = %v", reasons)
	}
}

func TestMatchVerdictsToleratesIDVariants(t *testing.T) {
	d := baseDocs()
	claims := d.claims()
	var letter string
	for _, c := range claims {
		if strings.HasPrefix(c.ID, "letter.paragraphs[") {
			letter = c.ID
			break
		}
	}
	if letter == "" {
		t.Fatal("baseDocs has no letter paragraph")
	}
	variant := strings.Replace(strings.Replace(letter, "paragraphs[", "Paragraph.", 1), "]", "", 1)
	got, unknown := matchVerdicts(claims, []Verdict{
		{ID: variant, Status: "supported"},
		{ID: "`" + claims[0].ID + "`", Status: "supported"},
		{ID: "cv.nonsense[9]", Status: "supported"},
	})
	if len(got) != 2 || got[0].ID != letter || got[1].ID != claims[0].ID {
		t.Fatalf("matched = %+v", got)
	}
	if len(unknown) != 1 || unknown[0] != "cv.nonsense[9]" {
		t.Fatalf("unknown = %v", unknown)
	}
}

func TestFinalizeFillsAndGroundsCoverage(t *testing.T) {
	prof := profile.Parse(fillProfile)
	rv := newDraftReview(prof, analysis)
	d := minimalDocs()
	w := rv.Finalize(&d, VerificationReport{Coverage: []Coverage{
		{Requirement: "Go", Covered: true, Evidence: "Programming: Go, SQL"},
		{Requirement: "Kubernetes", Covered: true, Evidence: "Ran Kubernetes clusters"},
	}})
	if len(d.CV.Skills) == 0 || len(d.CV.Languages) == 0 || len(w) != 2 {
		t.Fatalf("skills=%v languages=%v warnings=%v", d.CV.Skills, d.CV.Languages, w)
	}
	if !d.Coverage[0].Covered || d.Coverage[1].Covered {
		t.Fatalf("coverage = %+v", d.Coverage)
	}
}

const linuxProfile = `# Ada Example
ada@example.com

## Experience
### Backend developer, Initech (2020-2023)
- Built REST APIs in Go for the billing team

## Education
### BSc Computer Science, Northfield (2016-2020)
- Systems and security: Operating Systems, Networks, Cryptography and Security

## Skills
- Programming: Go, Python
- Systems and networking: Linux/Bash, Wireshark, OpenWrt
`

var linuxAnalysis = JobAnalysis{
	Company: "Acme", Role: "Linux engineer", Language: "en",
	Keywords: []KeyTerm{
		{Term: "Linux", Importance: ImportanceRequired},
		{Term: "Security", Importance: ImportanceRequired},
	},
}

func linuxDocs() Documents {
	d := baseDocs()
	d.CV.Skills = []Claim{
		{Text: "Programming: Go, Python", Sources: []string{"Programming: Go, Python"}},
		{Text: "Systems & Linux: Linux/Bash, Wireshark, OpenWrt, Security", Sources: []string{"Systems and networking: Linux/Bash, Wireshark, OpenWrt"}},
	}
	return d
}

// One unsupported item in a Skills line removes that item, not the line.
func TestGiveUpCutsOnlyTheUnsupportedSkill(t *testing.T) {
	rv := newDraftReview(profile.Parse(linuxProfile), linuxAnalysis)
	d := linuxDocs()
	f := findings{Issues: map[string]string{"cv.skills[1]": "unsupported: Security is only coursework"}}
	dropped, warnings := rv.GiveUp(&d, f)

	if len(d.CV.Skills) != 2 || d.CV.Skills[1].Text != "Systems & Linux: Linux/Bash, Wireshark, OpenWrt" {
		t.Fatalf("skills = %+v", d.CV.Skills)
	}
	if got := d.CV.Skills[1].Sources; len(got) != 1 || got[0] != "Systems and networking: Linux/Bash, Wireshark, OpenWrt" {
		t.Fatalf("sources = %v", got)
	}
	if len(dropped) != 1 || dropped[0].ID != "cv.skills[1]" || dropped[0].Text != "Security" || !strings.Contains(dropped[0].Reason, "only this was removed") {
		t.Fatalf("dropped = %+v", dropped)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
}

// A Skills line with nothing the profile lists, or nothing to remove, is
// dropped as before.
func TestGiveUpDropsSkillLinesItCannotRepair(t *testing.T) {
	rv := newDraftReview(profile.Parse(linuxProfile), linuxAnalysis)
	for _, text := range []string{"Expert in: Kubernetes, Terraform", "Systems & Linux: Linux/Bash, Wireshark"} {
		d := linuxDocs()
		d.CV.Skills[1].Text = text
		dropped, _ := rv.GiveUp(&d, findings{Issues: map[string]string{"cv.skills[1]": "unsupported: x"}})
		if len(d.CV.Skills) != 1 || len(dropped) != 1 || dropped[0].Text != "Skills: "+text {
			t.Fatalf("%q: skills = %+v, dropped = %+v", text, d.CV.Skills, dropped)
		}
	}
}

// Losing a supported required keyword to a removal is reported.
func TestGiveUpWarnsWhenARequiredKeywordIsLost(t *testing.T) {
	rv := newDraftReview(profile.Parse(linuxProfile), linuxAnalysis)
	d := linuxDocs()
	d.CV.Skills[1].Text = "Expert in: Linux, Kubernetes"
	f := findings{
		Issues:  map[string]string{"cv.skills[1]": "unsupported: x"},
		support: []KeywordSupport{{Term: "Linux", Supported: true, Evidence: "Linux/Bash, Wireshark, OpenWrt"}},
	}
	_, warnings := rv.GiveUp(&d, f)
	if len(warnings) != 1 || !strings.Contains(warnings[0], `"Linux"`) {
		t.Fatalf("warnings = %v", warnings)
	}
}

// A keyword backed only by a course name is reported, but the writer is not
// asked to add it to a Skills line or a bullet.
func TestCourseworkKeywordIsNotAskedFor(t *testing.T) {
	prof := profile.Parse(linuxProfile)
	d := baseDocs()
	support := []KeywordSupport{
		{Term: "Linux", Supported: true, Evidence: "Linux/Bash, Wireshark, OpenWrt"},
		{Term: "Security", Supported: true, Evidence: "Cryptography and Security"},
	}
	ats, gaps := keywordStatus(prof, &d, linuxAnalysis, support)
	if _, ok := gaps["Security"]; ok {
		t.Fatalf("gaps = %v", gaps)
	}
	if _, ok := gaps["Linux"]; !ok {
		t.Fatalf("a keyword from the Skills section is still asked for: %v", gaps)
	}
	if !slices.Contains(ats.Missing, "Security") {
		t.Fatalf("missing = %v", ats.Missing)
	}
}

// "Linux/Bash" names Linux for the keyword count, so dropping the only line
// holding it is reported.
func TestSlashSeparatesKeywords(t *testing.T) {
	rv := newDraftReview(profile.Parse(linuxProfile), linuxAnalysis)
	d := linuxDocs()
	d.CV.Skills[1].Text = "Linux/Bash, Wireshark" // no label: cannot be salvaged
	f := findings{
		Issues:  map[string]string{"cv.skills[1]": "unsupported: x"},
		support: []KeywordSupport{{Term: "Linux", Supported: true, Evidence: "Linux/Bash, Wireshark, OpenWrt"}},
	}
	if !appearsIn(cvText(&d), KeyTerm{Term: "Linux"}) {
		t.Fatal(`"Linux/Bash" must count as Linux`)
	}
	if _, w := rv.GiveUp(&d, f); len(w) != 1 {
		t.Fatalf("warnings = %v", w)
	}
}
