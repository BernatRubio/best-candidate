package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BernatRubio/best-candidate/internal/tailor"
)

func bullets(prefix string, n int) []tailor.Claim {
	var out []tailor.Claim
	for i := 1; i <= n; i++ {
		out = append(out, tailor.Claim{Text: fmt.Sprintf("%s bullet %d with a fair amount of detail about what was done, how, and with which tools.", prefix, i)})
	}
	return out
}

func longResult(jobs, bulletsPerJob int) *tailor.Result {
	res := sample()
	res.Docs.CV.Experience = nil
	for i := 1; i <= jobs; i++ {
		res.Docs.CV.Experience = append(res.Docs.CV.Experience, tailor.Experience{
			Title: fmt.Sprintf("Role %d", i), Company: fmt.Sprintf("Company %d", i), Period: "2020-2023",
			Bullets: bullets(fmt.Sprintf("job%d", i), bulletsPerJob),
		})
	}
	return res
}

func TestTrimCVOrderAndLimits(t *testing.T) {
	cv := &tailor.CV{
		Summary: tailor.Claim{Text: "A summary."},
		Skills:  bullets("skill", 4),
		Projects: []tailor.Project{
			{Name: "P", Details: bullets("proj", 4)},
		},
		Experience: []tailor.Experience{
			{Title: "New", Company: "A", Bullets: bullets("new", 4)},
			{Title: "Old", Company: "B", Bullets: bullets("old", 3)},
		},
		Education: []tailor.Education{
			{Degree: "D1", Details: bullets("edu1", 3)},
			{Degree: "D2", Details: bullets("edu2", 1)},
		},
		Sections: []tailor.Section{{Title: "Certifications", Items: bullets("cert", 2)}},
	}

	var notes []string
	for i := 0; i < 100; i++ {
		note, ok := trimCV(cv)
		if !ok {
			break
		}
		notes = append(notes, note)
	}
	if len(notes) == 0 {
		t.Fatal("nothing trimmed")
	}
	// Extras go first, the summary goes last.
	if !strings.Contains(notes[0], "Certifications") {
		t.Errorf("first cut = %q", notes[0])
	}
	if !strings.Contains(notes[len(notes)-1], "summary") {
		t.Errorf("last cut = %q", notes[len(notes)-1])
	}
	// Whole jobs and degrees are never removed; each keeps its most relevant (first) bullet.
	if len(cv.Experience) != 2 || len(cv.Education) != 2 || len(cv.Projects) != 1 {
		t.Fatalf("entries were removed: %+v", cv)
	}
	if got := cv.Experience[0].Bullets; len(got) != 1 || !strings.Contains(got[0].Text, "new bullet 1") {
		t.Errorf("job 1 = %+v", got)
	}
	if got := cv.Experience[1].Bullets; len(got) != 1 || !strings.Contains(got[0].Text, "old bullet 1") {
		t.Errorf("job 2 = %+v", got)
	}
	if got := cv.Education[0].Details; len(got) != 1 || !strings.Contains(got[0].Text, "edu1 bullet 1") {
		t.Errorf("degree 1 = %+v", got)
	}
	if len(cv.Skills) != 2 {
		t.Errorf("skills lines = %d, want 2", len(cv.Skills))
	}
	// Once everything is minimal, trimming reports that it cannot go on.
	if _, ok := trimCV(cv); ok {
		t.Error("trimCV should be exhausted")
	}
}

func TestBuildKeepsLongCVOnOnePage(t *testing.T) {
	if _, err := findTypst(); err != nil {
		t.Skip("typst not installed")
	}
	res := longResult(4, 12) // far more than a page
	dir := t.TempDir()
	info, err := Build(dir, res, time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if info.CVPages != 1 {
		t.Fatalf("cv has %d pages after trimming", info.CVPages)
	}
	if got := pdfPages(t, filepath.Join(dir, "cv.pdf")); got != 1 {
		t.Fatalf("cv.pdf has %d pages", got)
	}
	if len(info.Trimmed) == 0 {
		t.Fatal("a 48-bullet CV must have been trimmed")
	}
	// The most relevant (first) bullet of the first job survives; the tail was cut.
	first := res.Docs.CV.Experience[0].Bullets
	if len(first) == 0 || !strings.Contains(first[0].Text, "job1 bullet 1") {
		t.Fatalf("first bullet lost: %+v", first)
	}
	if len(first) >= 12 {
		t.Fatalf("nothing was cut from job 1: %d bullets", len(first))
	}
	report, _ := os.ReadFile(filepath.Join(dir, "report.md"))
	if !strings.Contains(string(report), "to keep the CV on one page") {
		t.Errorf("report does not mention the trimming:\n%s", report)
	}
}

func TestBuildLeavesAShortCVAlone(t *testing.T) {
	if _, err := findTypst(); err != nil {
		t.Skip("typst not installed")
	}
	res := sample()
	info, err := Build(t.TempDir(), res, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if info.CVPages != 1 || info.LetterPages != 1 || len(info.Trimmed) != 0 {
		t.Fatalf("info = %+v", info)
	}
}

// A CV that cannot be made to fit without dropping whole jobs is reported, not
// silently cut further.
func TestBuildReportsAnUnfittableCV(t *testing.T) {
	if _, err := findTypst(); err != nil {
		t.Skip("typst not installed")
	}
	res := longResult(45, 1)
	dir := t.TempDir()
	info, err := Build(dir, res, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if info.CVPages < 2 {
		t.Fatalf("expected an overlong CV, got %d page(s)", info.CVPages)
	}
	if len(res.Docs.CV.Experience) != 45 {
		t.Fatalf("jobs were dropped: %d left", len(res.Docs.CV.Experience))
	}
	if !strings.Contains(strings.Join(res.Warnings, " | "), "nothing more can be cut") {
		t.Fatalf("warnings = %v", res.Warnings)
	}
}

func TestBuildReportsALongLetter(t *testing.T) {
	if _, err := findTypst(); err != nil {
		t.Skip("typst not installed")
	}
	res := sample()
	long := strings.Repeat("This paragraph is deliberately long so that the letter cannot fit on one page. ", 40)
	res.Docs.Letter.Paragraphs = nil
	for i := 0; i < 6; i++ {
		res.Docs.Letter.Paragraphs = append(res.Docs.Letter.Paragraphs, tailor.Claim{Text: long})
	}
	info, err := Build(t.TempDir(), res, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if info.LetterPages < 2 {
		t.Fatalf("letter pages = %d", info.LetterPages)
	}
	if !strings.Contains(strings.Join(res.Warnings, " | "), "cover letter is") {
		t.Fatalf("warnings = %v", res.Warnings)
	}
	if info.CVPages != 1 {
		t.Fatalf("the CV must be unaffected: %d pages", info.CVPages)
	}
}
