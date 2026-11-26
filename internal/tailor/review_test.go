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
