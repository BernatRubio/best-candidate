// Command best-candidate turns a job offer and a master profile into a
// tailored CV and cover letter.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	bestcandidate "github.com/BernatRubio/best-candidate"
	"github.com/BernatRubio/best-candidate/internal/joboffer"
	"github.com/BernatRubio/best-candidate/internal/llm"
	"github.com/BernatRubio/best-candidate/internal/profile"
	"github.com/BernatRubio/best-candidate/internal/render"
	"github.com/BernatRubio/best-candidate/internal/tailor"
)

// errDropped signals a finished run in which unverifiable claims were removed.
var errDropped = errors.New("some statements could not be verified and were removed")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	root := &cobra.Command{
		Use:           "best-candidate",
		Short:         "Generate a tailored CV and cover letter from a job offer, without lying",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(initCmd(), generateCmd(), renderCmd())

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		if errors.Is(err, errDropped) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func initCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write a profile template to fill in",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists; not overwriting it", path)
			}
			if err := os.WriteFile(path, []byte(bestcandidate.ExampleProfile), 0o600); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s. Fill it in, then run:\n  best-candidate generate --job <file|url|->\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "profile", "profile.md", "where to write the profile")
	return cmd
}

func generateCmd() *cobra.Command {
	var (
		profilePath, job, lang, outDir string
		providerName, model, baseURL   string
		maxRepairs                     int
	)
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a CV and cover letter for a job offer",
		Long: `Reads your profile and a job offer, writes a tailored CV and cover letter as PDFs,
and checks every statement against your profile. Statements that cannot be verified are
removed and listed in report.md.

The job offer can be a file, an http(s) URL, or "-" for stdin.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if job == "" {
				return errors.New("--job is required (file, URL, or - for stdin)")
			}
			ctx := cmd.Context()
			logf := func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }

			prof, err := profile.Load(profilePath)
			if err != nil {
				return err
			}
			offer, err := joboffer.Load(ctx, job, os.Stdin)
			if err != nil {
				return err
			}
			provider, err := llm.FromEnv(llm.Config{Provider: providerName, Model: model, BaseURL: baseURL})
			if err != nil {
				return err
			}
			provider = llm.WithProgress(provider, logf)
			logf("using %s", provider.Name())

			res, err := tailor.Run(ctx, provider, prof, offer, tailor.Options{
				Lang: lang, MaxRepairs: maxRepairs, Logf: logf,
			})
			if err != nil {
				return err
			}

			now := time.Now()
			dir := filepath.Join(outDir, render.Slug(res.Analysis.Company, res.Analysis.Role, now.Format("2006-01-02")))
			if err := render.Write(dir, res, now); err != nil {
				return err
			}
			if err := render.Compile(dir); err != nil {
				return fmt.Errorf("documents written to %s but PDF build failed: %w", dir, err)
			}

			covered := 0
			for _, c := range res.Docs.Coverage {
				if c.Covered {
					covered++
				}
			}
			fmt.Printf("\nDone: %s\n", dir)
			fmt.Printf("  cv.pdf, cover-letter.pdf, report.md (plus editable cv.json, letter.json, *.typ)\n")
			fmt.Printf("  requirements covered by your profile: %d of %d\n", covered, len(res.Docs.Coverage))
			for _, w := range res.Warnings {
				fmt.Printf("  warning: %s\n", w)
			}
			if len(res.Dropped) > 0 {
				fmt.Printf("  %d statement(s) removed because they could not be verified; see report.md\n", len(res.Dropped))
				return errDropped
			}
			fmt.Println("  Read both documents before sending: verification is automated, not infallible.")
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&profilePath, "profile", "profile.md", "path to your master profile")
	f.StringVar(&job, "job", "", "job offer: file path, http(s) URL, or - for stdin")
	f.StringVar(&lang, "lang", "auto", "output language (ISO 639-1, e.g. en, es, ca) or auto to follow the offer")
	f.StringVar(&outDir, "out", "out", "directory for generated applications")
	f.StringVar(&providerName, "provider", "anthropic", "model provider: "+llm.Supported+" (env BC_PROVIDER overrides)")
	f.StringVar(&model, "model", "", "model id (env BC_MODEL overrides; required except for anthropic, which defaults to claude-opus-5-5)")
	f.StringVar(&baseURL, "base-url", "", "API base URL for ollama and openai-compatible (env BC_BASE_URL overrides)")
	f.IntVar(&maxRepairs, "max-repairs", 2, "correction rounds before unverifiable statements are dropped")
	return cmd
}

func renderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "render <dir>",
		Short: "Rebuild the PDFs in a generated folder after editing its json or typ files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := render.Compile(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Rebuilt %s/cv.pdf and %s/cover-letter.pdf\n", args[0], args[0])
			return nil
		},
	}
}
