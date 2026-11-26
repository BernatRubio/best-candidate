package render

import (
	"fmt"
	"strings"

	"github.com/BernatRubio/best-candidate/internal/tailor"
)

// Report renders report.md: the offer, the language and any warnings.
func Report(res *tailor.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Application report: %s", res.Role)
	if res.Company != "" {
		fmt.Fprintf(&b, " at %s", res.Company)
	}
	fmt.Fprintf(&b, "\n\nDocument language: `%s`\n", res.Lang)

	if len(res.Warnings) > 0 {
		b.WriteString("\n## Warnings\n\n")
		for _, w := range res.Warnings {
			fmt.Fprintf(&b, "- %s\n", w)
		}
	}

	b.WriteString("\n## Before you send\n\n")
	b.WriteString("Nothing was verified automatically. Read both documents and check every statement against what you actually did.\n")
	return b.String()
}
