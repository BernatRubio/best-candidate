package render

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BernatRubio/best-candidate/internal/tailor"
)

// pdfChunks returns the raw PDF and every zlib-decompressed stream in it: page
// and annotation objects may live inside compressed object streams.
func pdfChunks(data []byte) [][]byte {
	chunks := [][]byte{data}
	for _, m := range regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`).FindAllSubmatch(data, -1) {
		if r, err := zlib.NewReader(bytes.NewReader(m[1])); err == nil {
			if plain, err := io.ReadAll(r); err == nil {
				chunks = append(chunks, plain)
			}
		}
	}
	return chunks
}

var pageObject = regexp.MustCompile(`/Type\s*/Page[^s]`)

// PDFPages counts the pages of a PDF file.
func PDFPages(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range pdfChunks(data) {
		n += len(pageObject.FindAll(c, -1))
	}
	if n == 0 {
		return 0, errors.New("no pages found in " + path)
	}
	return n, nil
}

// largest returns the index of the entry with the most items among those with
// more than min (the later one on ties), or -1 if there is none.
func largest(counts []int, min int) int {
	best := -1
	for i, n := range counts {
		if n > min && (best < 0 || n >= counts[best]) {
			best = i
		}
	}
	return best
}

// trimCV removes one unit of content, least important first, and says what it
// removed. Lists are ordered by relevance (the model is told to put the most
// relevant item first), so cutting from the end loses the least. It only ever
// removes: nothing is added or reworded, so the CV stays truthful. It returns
// false when nothing more can be cut without dropping whole jobs or degrees.
func trimCV(cv *tailor.CV) (string, bool) {
	// 1. extra blocks such as certifications
	if n := len(cv.Sections); n > 0 {
		title := cv.Sections[n-1].Title
		if items := cv.Sections[n-1].Items; len(items) > 1 {
			cv.Sections[n-1].Items = items[:len(items)-1]
			return fmt.Sprintf("removed the last line of %q", title), true
		}
		cv.Sections = cv.Sections[:n-1]
		return fmt.Sprintf("removed the extra section %q", title), true
	}
	// 2. education: keep one line per degree
	counts := make([]int, len(cv.Education))
	for i, e := range cv.Education {
		counts[i] = len(e.Details)
	}
	if i := largest(counts, 1); i >= 0 {
		e := &cv.Education[i]
		e.Details = e.Details[:len(e.Details)-1]
		return fmt.Sprintf("removed an extra line from %q", e.Degree), true
	}
	// 3. projects beyond two bullets, 4. jobs beyond two bullets
	counts = make([]int, len(cv.Projects))
	for i, p := range cv.Projects {
		counts[i] = len(p.Details)
	}
	if i := largest(counts, 2); i >= 0 {
		p := &cv.Projects[i]
		p.Details = p.Details[:len(p.Details)-1]
		return fmt.Sprintf("removed a bullet from project %q", p.Name), true
	}
	counts = make([]int, len(cv.Experience))
	for i, e := range cv.Experience {
		counts[i] = len(e.Bullets)
	}
	if i := largest(counts, 2); i >= 0 {
		e := &cv.Experience[i]
		e.Bullets = e.Bullets[:len(e.Bullets)-1]
		return fmt.Sprintf("removed a bullet from %q at %q", e.Title, e.Company), true
	}
	// 5. down to one bullet per project and per job
	counts = make([]int, len(cv.Projects))
	for i, p := range cv.Projects {
		counts[i] = len(p.Details)
	}
	if i := largest(counts, 1); i >= 0 {
		p := &cv.Projects[i]
		p.Details = p.Details[:len(p.Details)-1]
		return fmt.Sprintf("removed a bullet from project %q", p.Name), true
	}
	counts = make([]int, len(cv.Experience))
	for i, e := range cv.Experience {
		counts[i] = len(e.Bullets)
	}
	if i := largest(counts, 1); i >= 0 {
		e := &cv.Experience[i]
		e.Bullets = e.Bullets[:len(e.Bullets)-1]
		return fmt.Sprintf("removed a bullet from %q at %q", e.Title, e.Company), true
	}
	// 6. skills beyond two lines (they carry the most keywords per line, so they go late)
	if len(cv.Skills) > 2 {
		cv.Skills = cv.Skills[:len(cv.Skills)-1]
		return "removed the last skills line", true
	}
	// 7. last resort: the summary
	if strings.TrimSpace(cv.Summary.Text) != "" {
		cv.Summary = tailor.Claim{}
		return "removed the profile summary", true
	}
	return "", false
}

// Fit cuts content from cv, least important first (see trimCV), until pages
// reports a single page or nothing more can be cut. pages renders the current
// cv and counts its pages. Fit returns the last count and a note per cut.
func Fit(cv *tailor.CV, pages func() (int, error)) (int, []string, error) {
	var cuts []string
	for {
		n, err := pages()
		if err != nil || n <= 1 {
			return n, cuts, err
		}
		note, ok := trimCV(cv)
		if !ok {
			return n, cuts, nil
		}
		cuts = append(cuts, note)
	}
}

// BuildResult describes what Build produced.
type BuildResult struct {
	CVPages     int
	LetterPages int
	Trimmed     []string // what was cut to keep the CV on one page
}

// Build writes all files, compiles both PDFs and guarantees a one-page CV:
// while cv.pdf has more than one page, the least important content is removed
// (see trimCV) and the CV is rendered again. Every cut is added to res.Warnings
// and so appears in report.md. A cover letter longer than one page is reported,
// not cut, because its paragraphs cannot be dropped independently.
func Build(dir string, res *tailor.Result, now time.Time) (BuildResult, error) {
	var out BuildResult
	pages := func() (int, error) {
		if err := Write(dir, res, now); err != nil {
			return 0, err
		}
		if err := compileOne(dir, "cv.typ", "cv.pdf"); err != nil {
			return 0, err
		}
		return PDFPages(filepath.Join(dir, "cv.pdf"))
	}
	n, cuts, err := Fit(&res.Docs.CV, pages)
	if err != nil {
		return out, err
	}
	out.CVPages, out.Trimmed = n, cuts
	for _, c := range cuts {
		res.Warnings = append(res.Warnings, "to keep the CV on one page: "+c)
	}
	if n > 1 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("the CV is %d pages and nothing more can be cut automatically; shorten cv.json by hand and run render", n))
	}

	if err := compileOne(dir, "letter.typ", "cover-letter.pdf"); err != nil {
		return out, err
	}
	lp, err := PDFPages(filepath.Join(dir, "cover-letter.pdf"))
	if err != nil {
		return out, err
	}
	out.LetterPages = lp
	if lp > 1 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("the cover letter is %d pages; shorten letter.json by hand and run render", lp))
	}
	// Rewrite so report.md lists every warning, including those found while compiling.
	return out, Write(dir, res, now)
}
