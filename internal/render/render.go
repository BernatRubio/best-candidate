// Package render writes the generated documents to disk and compiles them to
// PDF with Typst.
package render

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BernatRubio/best-candidate/internal/tailor"
)

//go:embed templates/common.typ templates/cv.typ templates/letter.typ
var templates embed.FS

// templateFiles are written next to the data files; common.typ is imported by the other two.
var templateFiles = []string{"common.typ", "cv.typ", "letter.typ"}

// CVFile is the data file read by cv.typ.
type CVFile struct {
	Lang string    `json:"lang"`
	CV   tailor.CV `json:"cv"`
}

// LetterFile is the data file read by letter.typ.
type LetterFile struct {
	Lang    string        `json:"lang"`
	Name    string        `json:"name"`
	Contact []string      `json:"contact"`
	Date    string        `json:"date"`
	Letter  tailor.Letter `json:"letter"`
}

// Write stores data files, templates and report in dir. Existing files are
// overwritten.
func Write(dir string, res *tailor.Result, now time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	docCV, docLetter := nonNil(res.Docs.CV, res.Docs.Letter)
	docCV.Contact = CleanContact(docCV.Contact)
	cv := CVFile{Lang: res.Lang, CV: docCV}
	letter := LetterFile{
		Lang:    res.Lang,
		Name:    docCV.Name,
		Contact: docCV.Contact,
		Date:    FormatDate(now, res.Lang),
		Letter:  docLetter,
	}
	if err := writeJSON(filepath.Join(dir, "cv.json"), cv); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "letter.json"), letter); err != nil {
		return err
	}
	for _, name := range templateFiles {
		if err := writeTemplate(dir, name, true); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(Report(res)), 0o644)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeTemplate(dir, name string, overwrite bool) error {
	path := filepath.Join(dir, name)
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}
	data, err := templates.ReadFile("templates/" + name)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Compile builds cv.pdf and cover-letter.pdf in dir. Missing templates are
// restored; existing (possibly hand-edited) ones are left alone.
func Compile(dir string) error {
	for _, name := range templateFiles {
		if err := writeTemplate(dir, name, false); err != nil {
			return err
		}
	}
	typst, err := findTypst()
	if err != nil {
		return err
	}
	for _, job := range [][2]string{{"cv.typ", "cv.pdf"}, {"letter.typ", "cover-letter.pdf"}} {
		cmd := exec.Command(typst, "compile", job[0], job[1])
		cmd.Dir = dir
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("typst compile %s: %w\n%s", job[0], err, strings.TrimSpace(stderr.String()))
		}
	}
	return nil
}

func findTypst() (string, error) {
	if p, err := exec.LookPath("typst"); err == nil {
		return p, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".cargo", "bin", "typst")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("typst not found: install it (https://typst.app) and make sure it is on your PATH")
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slug makes a short filesystem-safe name from free text.
func Slug(parts ...string) string {
	s := strings.ToLower(strings.Join(parts, " "))
	s = strings.Map(func(r rune) rune {
		switch r {
		case 'á', 'à', 'ä', 'â':
			return 'a'
		case 'é', 'è', 'ë', 'ê':
			return 'e'
		case 'í', 'ì', 'ï', 'î':
			return 'i'
		case 'ó', 'ò', 'ö', 'ô':
			return 'o'
		case 'ú', 'ù', 'ü', 'û':
			return 'u'
		case 'ñ':
			return 'n'
		case 'ç':
			return 'c'
		}
		return r
	}, s)
	s = strings.Trim(nonSlug.ReplaceAllString(s, "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	return s
}

var months = map[string][12]string{
	"en": {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	"es": {"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
	"ca": {"de gener", "de febrer", "de març", "d'abril", "de maig", "de juny", "de juliol", "d'agost", "de setembre", "d'octubre", "de novembre", "de desembre"},
}

// FormatDate writes a long date in the document language.
func FormatDate(t time.Time, lang string) string {
	m, ok := months[lang]
	if !ok {
		return t.Format("2006-01-02")
	}
	name := m[t.Month()-1]
	switch lang {
	case "en":
		return fmt.Sprintf("%s %d, %d", name, t.Day(), t.Year())
	case "es":
		return fmt.Sprintf("%d de %s de %d", t.Day(), name, t.Year())
	default: // ca
		return fmt.Sprintf("%d %s de %d", t.Day(), name, t.Year())
	}
}

func nn[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// nonNil replaces nil slices with empty ones so the JSON has [] rather than
// null, which the Typst templates cannot take the length of.
func nonNilClaims(cs []tailor.Claim) []tailor.Claim {
	out := make([]tailor.Claim, 0, len(cs))
	for _, c := range cs {
		c.Sources = nn(c.Sources)
		out = append(out, c)
	}
	return out
}

func nonNil(cv tailor.CV, l tailor.Letter) (tailor.CV, tailor.Letter) {
	cv.Contact = nn(cv.Contact)
	cv.Summary.Sources = nn(cv.Summary.Sources)
	cv.Skills = nonNilClaims(cv.Skills)
	cv.Languages = nonNilClaims(cv.Languages)
	projs := make([]tailor.Project, 0, len(cv.Projects))
	for _, p := range cv.Projects {
		p.Sources = nn(p.Sources)
		p.Details = nonNilClaims(p.Details)
		projs = append(projs, p)
	}
	cv.Projects = projs
	exps := make([]tailor.Experience, 0, len(cv.Experience))
	for _, e := range cv.Experience {
		e.Sources = nn(e.Sources)
		bullets := make([]tailor.Claim, 0, len(e.Bullets))
		for _, b := range e.Bullets {
			b.Sources = nn(b.Sources)
			bullets = append(bullets, b)
		}
		e.Bullets = bullets
		exps = append(exps, e)
	}
	cv.Experience = exps
	edus := make([]tailor.Education, 0, len(cv.Education))
	for _, e := range cv.Education {
		e.Sources = nn(e.Sources)
		details := make([]tailor.Claim, 0, len(e.Details))
		for _, b := range e.Details {
			b.Sources = nn(b.Sources)
			details = append(details, b)
		}
		e.Details = details
		edus = append(edus, e)
	}
	cv.Education = edus
	secs := make([]tailor.Section, 0, len(cv.Sections))
	for _, s := range cv.Sections {
		items := make([]tailor.Claim, 0, len(s.Items))
		for _, b := range s.Items {
			b.Sources = nn(b.Sources)
			items = append(items, b)
		}
		s.Items = items
		secs = append(secs, s)
	}
	cv.Sections = secs
	paras := make([]tailor.Claim, 0, len(l.Paragraphs))
	for _, p := range l.Paragraphs {
		p.Sources = nn(p.Sources)
		paras = append(paras, p)
	}
	l.Paragraphs = paras
	return cv, l
}

var (
	contactLabel = regexp.MustCompile(`(?i)^(location|address|email|e-mail|mail|phone|tel|telephone|mobile|links?|linkedin|github|website|web|portfolio|url)\s*:\s*`)
	webOrMail    = regexp.MustCompile(`(?i)(https?://|www\.|@)`)
	listSplit    = regexp.MustCompile(`\s*[,;]\s+`)
)

// CleanContact turns contact lines copied from the profile ("Email: a@b.c",
// "Links: www.x.com/in/me, https://github.com/me") into one plain entry each
// ("a@b.c", "www.x.com/in/me", "https://github.com/me"): labels are dropped and
// lists of links are split, so the templates can make each entry clickable.
// Lines like "Barcelona, Spain" are left whole.
func CleanContact(lines []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		label := ""
		if m := contactLabel.FindStringSubmatch(line); m != nil {
			label = strings.ToLower(m[1])
			line = strings.TrimSpace(line[len(m[0]):])
		}
		parts := []string{line}
		if label == "links" || webOrMail.MatchString(line) {
			parts = listSplit.Split(line, -1)
		}
		for _, p := range parts {
			p = strings.Trim(strings.TrimSpace(p), ",;")
			key := strings.ToLower(p)
			if p == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, p)
		}
	}
	return out
}

var parentheses = regexp.MustCompile(`\([^)]*\)`)

// shortSlug slugs s without parenthetical remarks and cuts it to at most max
// characters at a word boundary.
func shortSlug(s string, max int) string {
	slug := Slug(parentheses.ReplaceAllString(s, " "))
	if len(slug) <= max {
		return slug
	}
	cut := slug[:max]
	if i := strings.LastIndex(cut, "-"); i > 0 {
		cut = cut[:i]
	}
	return strings.Trim(cut, "-")
}

// FolderName names the output folder "<company>-<role>-<date>". Company and role
// are shortened separately so that a long, chatty value from the model can never
// push the date out of the name.
func FolderName(company, role string, t time.Time) string {
	var parts []string
	for _, p := range []string{shortSlug(company, 24), shortSlug(role, 32), t.Format("2006-01-02")} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "-")
}
