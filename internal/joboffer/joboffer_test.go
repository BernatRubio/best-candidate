package joboffer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStdin(t *testing.T) {
	got, err := Load(context.Background(), "-", strings.NewReader("  Senior Go developer \n"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "Senior Go developer" {
		t.Fatalf("got %q", got)
	}
}

func TestLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "offer.md")
	if err := os.WriteFile(path, []byte("# Backend engineer\nGo, Postgres"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Postgres") {
		t.Fatalf("got %q", got)
	}
}

func TestLoadEmpty(t *testing.T) {
	if _, err := Load(context.Background(), "-", strings.NewReader("  \n")); err == nil {
		t.Fatal("expected error for empty offer")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(context.Background(), "/nonexistent/offer.md", nil); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFetchURLHTML(t *testing.T) {
	const page = `<html><head><title>Platform Engineer at Acme</title></head><body>
<nav>Home | Jobs | About</nav>
<article><h1>Platform Engineer</h1>
<p>Acme is looking for a platform engineer to run our Kubernetes clusters and improve developer tooling across several product teams.</p>
<ul><li>3+ years with Kubernetes</li><li>Experience with Terraform</li></ul>
<p>You will work closely with the product teams, own the CI/CD pipeline, and help us grow a culture of reliability.</p>
</article></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	got, err := Load(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Kubernetes", "Terraform"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFetchURLNotFound(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if _, err := Load(context.Background(), srv.URL, nil); err == nil {
		t.Fatal("expected error for 404")
	}
}
