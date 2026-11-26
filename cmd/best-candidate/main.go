// Command best-candidate turns a job offer and a master profile into a
// tailored CV and cover letter.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	bestcandidate "github.com/BernatRubio/best-candidate"
	"github.com/BernatRubio/best-candidate/internal/dotenv"
	"github.com/BernatRubio/best-candidate/internal/joboffer"
	"github.com/BernatRubio/best-candidate/internal/llm"
	"github.com/BernatRubio/best-candidate/internal/profile"
	"github.com/BernatRubio/best-candidate/internal/render"
	"github.com/BernatRubio/best-candidate/internal/tailor"
)

func main() {
	// Settings such as BC_MODEL may live in a .env file in the folder the
	// command is run from; variables already set in the shell take precedence.
	if err := dotenv.Load(".env"); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not read .env:", err)
	}

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
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s. Fill it in, then run:\n  best-candidate generate <offer.md|url>\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "profile", "profile.md", "where to write the profile")
	return cmd
}

func generateCmd() *cobra.Command {
	var (
		profilePath, lang, outDir, model string
	)
	cmd := &cobra.Command{
		Use:   "generate <offer>",
		Short: "Generate a CV and cover letter for a job offer (markdown file or http(s) URL)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			job := args[0]
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			logf := func(format string, args ...any) { fmt.Fprintf(cmd.ErrOrStderr(), format+"\n", args...) }

			prof, err := profile.Load(profilePath)
			if err != nil {
				return err
			}
			offer, err := joboffer.Load(ctx, job)
			if err != nil {
				return err
			}
			provider := llm.WithProgress(llm.FromEnv(model), logf)
			logf("using %s", provider.Name())

			res, err := tailor.Run(ctx, provider, prof, offer, tailor.Options{
				Lang: lang, Logf: logf,
			})
			if err != nil {
				return err
			}

			now := time.Now()
			dir := filepath.Join(outDir, render.FolderName(res.Company, res.Role, now))
			info, err := render.Build(dir, res, now)
			if err != nil {
				return fmt.Errorf("documents written to %s but PDF build failed: %w", dir, err)
			}

			fmt.Fprintf(out, "\nDone: %s\n", dir)
			fmt.Fprintln(out, "  cv.pdf, cover-letter.pdf, report.md (plus editable cv.json, letter.json, *.typ)")
			fmt.Fprintf(out, "  cv.pdf: %d page(s), cover-letter.pdf: %d page(s)\n", info.CVPages, info.LetterPages)
			for _, w := range res.Warnings {
				fmt.Fprintf(out, "  warning: %s\n", w)
			}
			fmt.Fprintln(out, "  Read both documents before sending: nothing was verified automatically.")
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&profilePath, "profile", "profile.md", "path to your master profile")
	f.StringVar(&lang, "lang", "auto", "output language (ISO 639-1, e.g. en, es, ca) or auto to follow the offer")
	f.StringVar(&outDir, "out", "out", "directory for generated applications")
	f.StringVar(&model, "model", "", "Claude model, e.g. sonnet or opus (default: the claude CLI's; env BC_MODEL overrides)")
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
