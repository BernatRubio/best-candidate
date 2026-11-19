package render

import (
	"fmt"
	"strings"

	"github.com/BernatRubio/best-candidate/internal/tailor"
)

// Report renders the review document: what the profile matched, what it
// missed, and everything that was removed for lack of support.
func Report(res *tailor.Result) string {
	var b strings.Builder
	a := res.Analysis
	fmt.Fprintf(&b, "# Application report: %s", a.Role)
	if a.Company != "" {
		fmt.Fprintf(&b, " at %s", a.Company)
	}
	fmt.Fprintf(&b, "\n\nDocument language: `%s`\n\n", res.Lang)

	var matched, missed []tailor.Coverage
	for _, c := range res.Docs.Coverage {
		if c.Covered {
			matched = append(matched, c)
		} else {
			missed = append(missed, c)
		}
	}

	if len(res.Docs.Coverage) == 0 {
		b.WriteString("## Requirements\n\nThe model did not assess the offer's requirements, so nothing can be said about what your profile covers. Compare the offer with your profile yourself.\n\n")
	}
	b.WriteString("## Requirements the profile covers\n\n")
	if len(matched) == 0 {
		b.WriteString("None.\n")
	}
	for _, c := range matched {
		fmt.Fprintf(&b, "- **%s**: \"%s\"\n", c.Requirement, c.Evidence)
	}

	b.WriteString("\n## Requirements the profile does not show\n\n")
	b.WriteString("These are not mentioned in the CV or the letter. If you do have this experience, add it to your profile and run again.\n\n")
	if len(missed) == 0 {
		b.WriteString("None.\n")
	}
	for _, c := range missed {
		fmt.Fprintf(&b, "- %s\n", c.Requirement)
	}

	b.WriteString("\n## ATS keyword match\n\n")
	if total := len(res.ATS.Present) + len(res.ATS.Missing); total+len(res.ATS.NotInProfile) == 0 {
		b.WriteString("No keyword assessment is available.\n")
	} else {
		fmt.Fprintf(&b, "%d of %d offer keywords your profile supports appear in the CV.\n\n", len(res.ATS.Present), total)
		if len(res.ATS.Missing) > 0 {
			b.WriteString("Supported by your profile but not in the CV (they did not fit truthfully or were cut for space):\n\n")
			for _, k := range res.ATS.Missing {
				fmt.Fprintf(&b, "- %s\n", k)
			}
			b.WriteString("\n")
		}
		if len(res.ATS.NotInBullets) > 0 {
			b.WriteString("In the CV only as a skill, although your profile shows them used in your work:\n\n")
			for _, k := range res.ATS.NotInBullets {
				fmt.Fprintf(&b, "- %s\n", k)
			}
			b.WriteString("\n")
		}
		if len(res.ATS.NotInProfile) > 0 {
			b.WriteString("In the offer but not shown by your profile. If you do have this experience, add it to your profile and run again:\n\n")
			for _, k := range res.ATS.NotInProfile {
				fmt.Fprintf(&b, "- %s\n", k)
			}
		}
	}

	if res.ATS.Bullets > 0 {
		fmt.Fprintf(&b, "\n%d of %d experience and project bullets are word for word as in your profile (not tailored).\n", res.ATS.Unchanged, res.ATS.Bullets)
	}

	b.WriteString("\n## Removed because it could not be verified\n\n")
	if len(res.Dropped) == 0 {
		b.WriteString("Nothing was removed: every statement was backed by the profile.\n")
	} else {
		b.WriteString("The model kept making these claims after correction rounds, so they were taken out of the documents.\n\n")
		for _, d := range res.Dropped {
			fmt.Fprintf(&b, "- `%s`: %q\n  - %s\n", d.ID, d.Text, d.Reason)
		}
	}

	if len(res.Warnings) > 0 {
		b.WriteString("\n## Warnings\n\n")
		for _, w := range res.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
	}

	b.WriteString("\n## Before you send\n\n")
	b.WriteString("Verification is automated and not infallible. Read both documents and check every statement against what you actually did.\n")
	return b.String()
}
