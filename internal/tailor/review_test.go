package tailor

import (
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

	// A claim the reviewer skipped counts as unreviewed; a verdict is kept.
	d = baseDocs()
	f = rv.Review(&d, VerificationReport{Verdicts: []Verdict{{ID: "cv.experience[0]", Status: "exaggerated", Reason: "x"}}})
	if f.Issues["cv.experience[0].bullets[0]"] != "not reviewed by the fact-checker" || f.Issues["cv.experience[0]"] != "exaggerated: x" {
		t.Fatalf("issues = %v", f.Issues)
	}
	if !strings.Contains(f.problems(&d), "cv.experience[0]: ") {
		t.Fatalf("problems = %s", f.problems(&d))
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
