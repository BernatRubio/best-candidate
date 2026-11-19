package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bestcandidate "github.com/BernatRubio/best-candidate"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadStripsComments(t *testing.T) {
	path := write(t, `# Ada

<!-- note to self: add numbers -->

## Experience

- Built APIs in Go <!-- TODO: how many? -->

<!--
multi
line
-->

## Skills

- Go
`)
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "TODO") || strings.Contains(got, "note to self") || strings.Contains(got, "multi") {
		t.Fatalf("comments leaked:\n%s", got)
	}
	for _, want := range []string{"# Ada", "- Built APIs in Go", "- Go"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("blank lines not collapsed:\n%q", got)
	}
}

func TestLoadOnlyCommentsIsEmpty(t *testing.T) {
	if _, err := Load(write(t, "<!-- nothing here -->\n")); err == nil {
		t.Fatal("expected error for a profile that only holds comments")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("/nonexistent/profile.md"); err == nil {
		t.Fatal("expected error")
	}
}

// The shipped template is mostly guidance in comments: none of it may reach the model.
func TestExampleProfileGuidanceIsStripped(t *testing.T) {
	got := Clean(bestcandidate.ExampleProfile)
	if strings.Contains(got, "<!--") || strings.Contains(got, "Tips") {
		t.Fatalf("template comments leaked:\n%s", got)
	}
	if !strings.Contains(got, "## Experience") {
		t.Fatal("template content lost")
	}
}
