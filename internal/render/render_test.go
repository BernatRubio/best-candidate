package render

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/BernatRubio/best-candidate/internal/tailor"
)

func sample() *tailor.Result {
	return &tailor.Result{
		Lang:    "ca",
		Company: "Acme",
		Role:    "Desenvolupador Go",
		Docs: tailor.Documents{
			CV: tailor.CV{
				Name:    "Ada Exemple",
				Contact: []string{"Location: Barcelona", "Email: ada@example.com", "Phone: +34 600 000 000", "Links: www.linkedin.com/in/ada, https://github.com/ada"},
				Summary: tailor.Claim{Text: "Desenvolupadora backend amb experiència en Go."},
				Experience: []tailor.Experience{{
					Title: "Desenvolupadora backend", Company: "Initech", Location: "Barcelona", Period: "2020-2023",
					Bullets: []tailor.Claim{{Text: "Vaig crear APIs REST en Go per a l'equip de facturació."}},
				}},
				Education: []tailor.Education{{Degree: "Enginyeria Informàtica", Institution: "UPC", Period: "2016-2020"}},
				Skills:    []tailor.Claim{{Text: "Llenguatges: Go, Python"}, {Text: "Eines: Git, Docker"}},
				Languages: []tailor.Claim{{Text: "Català (nadiu)"}, {Text: "Anglès (B2)"}},
				Projects: []tailor.Project{{
					Name: "Orion", Link: "https://github.com/ada/orion",
					Details: []tailor.Claim{{Text: "Algorisme de descompressió en temps real. Documentació a https://example.com/docs."}},
				}},
				ProjectsFirst: true,
				Sections: []tailor.Section{
					{Title: "Certificacions", Layout: tailor.LayoutBullets, Placement: tailor.PlaceAfterEducation, Items: []tailor.Claim{{Text: "Cambridge B2 First"}}},
				},
			},
			Letter: tailor.Letter{
				Recipient: "Acme", Greeting: "Estimat equip,",
				Paragraphs: []tailor.Claim{{Text: "Us escric per al lloc de desenvolupador Go."}},
				Closing:    "Cordialment,", Signature: "Ada Exemple",
			},
		},
		Warnings: []string{"to keep the CV on one page: dropped a bullet"},
	}
}

func TestWriteAndCompile(t *testing.T) {
	if _, err := findTypst(); err != nil {
		t.Skip("typst not installed")
	}
	dir := t.TempDir()
	if err := Write(dir, sample(), time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := Compile(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cv.pdf", "cover-letter.pdf"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(data), "%PDF") {
			t.Fatalf("%s is not a PDF", name)
		}
	}
}

func TestCompileKeepsHandEditedTemplate(t *testing.T) {
	if _, err := findTypst(); err != nil {
		t.Skip("typst not installed")
	}
	dir := t.TempDir()
	if err := Write(dir, sample(), time.Now()); err != nil {
		t.Fatal(err)
	}
	custom := "// hand edited\n" + readFile(t, filepath.Join(dir, "cv.typ"))
	if err := os.WriteFile(filepath.Join(dir, "cv.typ"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Compile(dir); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, "cv.typ")); got != custom {
		t.Fatal("Compile overwrote an edited template")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSlugAndDate(t *testing.T) {
	if got := Slug("Acme S.L.", "Desenvolupador/a Go – Sènior"); got != "acme-s-l-desenvolupador-a-go-senior" {
		t.Fatalf("slug = %q", got)
	}
	d := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"en": "November 19, 2026",
		"es": "19 de noviembre de 2026",
		"ca": "19 de novembre de 2026",
		"fr": "2026-11-19",
	}
	for lang, want := range cases {
		if got := FormatDate(d, lang); got != want {
			t.Errorf("%s: %q != %q", lang, got, want)
		}
	}
}

// pdfURIs lists the link targets (/URI annotations) found in a PDF.
func pdfURIs(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	uris := map[string]bool{}
	re := regexp.MustCompile(`/URI\s*\(([^)]*)\)`)
	for _, c := range pdfChunks(data) {
		for _, m := range re.FindAllSubmatch(c, -1) {
			uris[string(m[1])] = true
		}
	}
	return uris
}

func TestLinksAreClickable(t *testing.T) {
	if _, err := findTypst(); err != nil {
		t.Skip("typst not installed")
	}
	dir := t.TempDir()
	if err := Write(dir, sample(), time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := Compile(dir); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"mailto:ada@example.com",
		"https://www.linkedin.com/in/ada",
		"https://github.com/ada",
	}
	for _, pdf := range []string{"cv.pdf", "cover-letter.pdf"} {
		got := pdfURIs(t, filepath.Join(dir, pdf))
		for _, w := range want {
			if !got[w] {
				t.Errorf("%s: missing link %q (have %v)", pdf, w, got)
			}
		}
		for u := range got {
			if strings.HasPrefix(u, "tel:") {
				t.Errorf("%s: phone numbers must stay plain text, found %q", pdf, u)
			}
		}
	}
	// A URL written inside a bullet is linked too (without the trailing full stop).
	if got := pdfURIs(t, filepath.Join(dir, "cv.pdf")); !got["https://example.com/docs"] {
		t.Errorf("body URL not linked: %v", got)
	}
}

func TestCleanContact(t *testing.T) {
	in := []string{
		"Location: Barcelona",
		"Email: ada@example.com",
		"Phone: +34 600 000 000",
		"Links: www.linkedin.com/in/ada, https://github.com/ada",
		"Barcelona, Spain",
		"EMAIL: ada@example.com", // duplicate, different case
		"",
	}
	want := []string{"Barcelona", "ada@example.com", "+34 600 000 000", "www.linkedin.com/in/ada", "https://github.com/ada", "Barcelona, Spain"}
	got := CleanContact(in)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if got := CleanContact(nil); got == nil || len(got) != 0 {
		t.Fatalf("nil input must give an empty, non-nil slice: %#v", got)
	}
}

func TestOldCVJSONStillRenders(t *testing.T) {
	if _, err := findTypst(); err != nil {
		t.Skip("typst not installed")
	}
	dir := t.TempDir()
	if err := Write(dir, sample(), time.Now()); err != nil {
		t.Fatal(err)
	}
	// Files written before `layout`/`placement` existed used `inline`.
	old := `{"lang":"en","cv":{"name":"Ada","contact":["a@b.co"],"summary":{"text":"","sources":[]},` +
		`"experience":[],"education":[],"sections":[{"title":"Skills","inline":true,"items":[{"text":"Go","sources":[]},{"text":"C","sources":[]}]}]}}`
	if err := os.WriteFile(filepath.Join(dir, "cv.json"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Compile(dir); err != nil {
		t.Fatal(err)
	}
}

// pdfPages counts the pages of a PDF.
func pdfPages(t *testing.T, path string) int {
	t.Helper()
	n, err := PDFPages(path)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func compileSample(t *testing.T, res *tailor.Result) string {
	t.Helper()
	if _, err := findTypst(); err != nil {
		t.Skip("typst not installed")
	}
	dir := t.TempDir()
	if err := Write(dir, res, time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := Compile(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A normal-sized CV and letter must each fit on one page: the templates shrink
// the font before they spill over.
func TestShortDocumentsFitOnePage(t *testing.T) {
	dir := compileSample(t, sample())
	for _, pdf := range []string{"cv.pdf", "cover-letter.pdf"} {
		if n := pdfPages(t, filepath.Join(dir, pdf)); n != 1 {
			t.Errorf("%s has %d pages, want 1", pdf, n)
		}
	}
}

// A CV that cannot fit one page at the smallest size flows onto a second page
// instead of being cut off or failing.
func TestVeryLongCVFlowsToSecondPage(t *testing.T) {
	res := sample()
	var bullets []tailor.Claim
	for i := 0; i < 60; i++ {
		bullets = append(bullets, tailor.Claim{Text: "A fairly long bullet describing one more thing that was done in this role, in some detail."})
	}
	res.Docs.CV.Experience[0].Bullets = bullets
	dir := compileSample(t, res)
	if n := pdfPages(t, filepath.Join(dir, "cv.pdf")); n < 2 {
		t.Errorf("cv.pdf has %d page(s); a 60-bullet CV must continue on a second page", n)
	}
}

// A CV with nothing optional filled in must still render (no null lists, no
// empty headings).
func TestMinimalCVRenders(t *testing.T) {
	res := &tailor.Result{Lang: "en", Docs: tailor.Documents{
		CV:     tailor.CV{Name: "Ada"},
		Letter: tailor.Letter{Greeting: "Hello", Closing: "Bye", Signature: "Ada"},
	}}
	dir := compileSample(t, res)
	if n := pdfPages(t, filepath.Join(dir, "cv.pdf")); n != 1 {
		t.Errorf("minimal cv has %d pages", n)
	}
}

func TestFolderName(t *testing.T) {
	d := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	cases := []struct{ company, role, want string }{
		{"Acme", "Software Engineer Intern", "acme-software-engineer-intern-2026-11-19"},
		// A chatty model answer must not push the date out of the name.
		{"Acme (part of Globex & Initech Holdings)",
			"Software Engineer Intern - Game Studio (position overview refers to the role as Mobile Engineer Intern)",
			"acme-software-engineer-intern-game-2026-11-19"},
		{"", "", "2026-11-19"},
		{"Acme", "", "acme-2026-11-19"},
	}
	for _, c := range cases {
		if got := FolderName(c.company, c.role, d); got != c.want {
			t.Errorf("FolderName(%q, %q) = %q, want %q", c.company, c.role, got, c.want)
		}
	}
}

func TestReport(t *testing.T) {
	r := Report(sample())
	for _, want := range []string{"Desenvolupador Go at Acme", "`ca`", "## Warnings", "dropped a bullet"} {
		if !strings.Contains(r, want) {
			t.Errorf("report lacks %q:\n%s", want, r)
		}
	}
}

func TestReportKeywords(t *testing.T) {
	res := sample()
	if strings.Contains(Report(res), "## Keywords") {
		t.Fatal("keyword section without keywords")
	}
	res.Keywords = []tailor.KeywordHit{
		{Keyword: tailor.Keyword{Term: "Go", Required: true, Supported: true}, Count: 2},
		{Keyword: tailor.Keyword{Term: "Docker", Supported: true}},
		{Keyword: tailor.Keyword{Term: "AWS"}},
		{Keyword: tailor.Keyword{Term: "Kubernetes", Required: true}},
	}
	r := Report(res)
	for _, want := range []string{
		"covers 1/2 supported keywords (1/1 required)",
		"### Covered", "- Go (×2) — required",
		"### Missed", "- Docker\n",
		"### Gaps", "- Kubernetes — required\n- AWS\n",
	} {
		if !strings.Contains(r, want) {
			t.Errorf("report lacks %q:\n%s", want, r)
		}
	}
}

func TestRecountAfterCut(t *testing.T) {
	res := sample()
	res.Keywords = []tailor.KeywordHit{{Keyword: tailor.Keyword{Term: "facturació", Supported: true}, Count: 1}}
	res.Docs.CV.Experience[0].Bullets = nil
	recount(res)
	if res.Keywords[0].Count != 0 || len(res.Warnings) == 0 || !strings.Contains(res.Warnings[len(res.Warnings)-1], "after cutting") {
		t.Fatalf("keywords = %+v, warnings = %q", res.Keywords, res.Warnings)
	}
}
