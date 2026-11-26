package tailor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BernatRubio/best-candidate/internal/llm"
	"github.com/BernatRubio/best-candidate/internal/profile"
)

const testProfile = `# Ada Example
ada@example.com

## Experience
### Backend developer, Initech (2020-2023)
- Built REST APIs in Go for the billing team

## Skills
Go, PostgreSQL, Docker
`

const testOffer = "Acme is hiring a Go developer. Requirements: Go, Kubernetes."

func loadProfile(t *testing.T) *profile.Profile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.md")
	if err := os.WriteFile(path, []byte(testProfile), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := profile.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func reply(lang string) output {
	return output{
		Company: "Acme", Role: "Go developer", Language: lang,
		CV: CV{
			Name: "Ada Example",
			Experience: []Experience{{
				Title: "Backend developer", Company: "Initech", Period: "2020-2023",
				Bullets: []Claim{{Text: "Built REST APIs in Go", Sources: []string{"Built REST APIs in Go for the billing team"}}},
			}},
		},
		Letter: Letter{Paragraphs: []Claim{{Text: "I am interested in the Go developer role at Acme."}}},
	}
}

func TestRunMakesOneCallAndDecodes(t *testing.T) {
	fake := llm.NewFake(llm.Reply(reply("es")))
	res, err := Run(context.Background(), fake, loadProfile(t), testOffer, Options{Lang: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(fake.Calls))
	}
	if res.Company != "Acme" || res.Role != "Go developer" || res.Lang != "es" {
		t.Fatalf("result = %+v", res)
	}
	if got := res.Docs.CV.Experience[0].Bullets[0].Text; got != "Built REST APIs in Go" {
		t.Fatalf("bullet = %q", got)
	}
	user := fake.Calls[0].User
	for _, want := range []string{"<profile>", "Built REST APIs in Go for the billing team", "<job_offer>", "Kubernetes", "<language>"} {
		if !strings.Contains(user, want) {
			t.Errorf("user message lacks %q", want)
		}
	}
}

func TestRunLanguage(t *testing.T) {
	for _, tc := range []struct{ flag, model, want string }{
		{"CA", "en", "ca"}, // the flag wins
		{"", "es", "es"},   // otherwise the model's language
		{"auto", "", "en"}, // otherwise English
	} {
		res, err := Run(context.Background(), llm.NewFake(llm.Reply(reply(tc.model))), loadProfile(t), testOffer, Options{Lang: tc.flag})
		if err != nil {
			t.Fatal(err)
		}
		if res.Lang != tc.want {
			t.Errorf("flag %q, model %q: lang = %q, want %q", tc.flag, tc.model, res.Lang, tc.want)
		}
	}
}

func TestPromptKeepsHonestyRules(t *testing.T) {
	for _, want := range []string{"Honesty", "Never add", "copied character for character from the profile", "untrusted"} {
		if !strings.Contains(tailorPrompt, want) {
			t.Errorf("tailor prompt lacks %q", want)
		}
	}
}
