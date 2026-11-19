package dotenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	got := Parse(`# a comment
BC_API_KEY=abc123
export BC_PROVIDER=openai-compatible
QUOTED="has spaces # not a comment"
SINGLE='single quoted'
WITH_COMMENT=value # trailing comment
EMPTY=
  SPACED_KEY  =  padded  
bad line without equals
1BAD=x
CRLF=windows` + "\r\n")
	want := map[string]string{
		"BC_API_KEY":   "abc123",
		"BC_PROVIDER":  "openai-compatible",
		"QUOTED":       "has spaces # not a comment",
		"SINGLE":       "single quoted",
		"WITH_COMMENT": "value",
		"EMPTY":        "",
		"SPACED_KEY":   "padded",
		"CRLF":         "windows",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestLoadDoesNotOverrideTheShell(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("DOTENV_TEST_A=from-file\nDOTENV_TEST_B=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_TEST_A", "from-shell")
	t.Setenv("DOTENV_TEST_B", "") // empty counts as unset
	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DOTENV_TEST_A"); got != "from-shell" {
		t.Errorf("shell value overridden: %q", got)
	}
	if got := os.Getenv("DOTENV_TEST_B"); got != "from-file" {
		t.Errorf("empty variable not filled: %q", got)
	}
}

func TestLoadMissingFileIsFine(t *testing.T) {
	if err := Load(filepath.Join(t.TempDir(), "nope.env")); err != nil {
		t.Fatal(err)
	}
}
