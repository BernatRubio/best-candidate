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
	writeKeywords(&b, res.Keywords)

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

// writeKeywords lists which offer keywords the CV covers, which supported ones
// it misses, and which the profile does not support (gaps).
func writeKeywords(b *strings.Builder, hits []tailor.KeywordHit) {
	if len(hits) == 0 {
		return
	}
	var covered, missed, gapsReq, gapsOpt []tailor.KeywordHit
	supported, required, requiredCovered := 0, 0, 0
	for _, h := range hits {
		switch {
		case !h.Supported && h.Required:
			gapsReq = append(gapsReq, h)
		case !h.Supported:
			gapsOpt = append(gapsOpt, h)
		case h.Count > 0:
			covered = append(covered, h)
		default:
			missed = append(missed, h)
		}
		if h.Supported {
			supported++
			if h.Required {
				required++
				if h.Count > 0 {
					requiredCovered++
				}
			}
		}
	}

	b.WriteString("\n## Keywords\n\n")
	fmt.Fprintf(b, "The CV covers %d/%d supported keywords (%d/%d required).\n",
		len(covered), supported, requiredCovered, required)
	if len(covered) > 0 {
		b.WriteString("\n### Covered\n\n")
		for _, h := range covered {
			fmt.Fprintf(b, "- %s (×%d)%s\n", h.Term, h.Count, requiredMark(h))
		}
	}
	if len(missed) > 0 {
		b.WriteString("\n### Missed\n\nSupported by the profile but not in the CV.\n\n")
		for _, h := range missed {
			fmt.Fprintf(b, "- %s%s\n", h.Term, requiredMark(h))
		}
	}
	if gaps := append(gapsReq, gapsOpt...); len(gaps) > 0 {
		b.WriteString("\n### Gaps\n\nAsked for by the offer but not shown in the profile. Add them to the profile only if they are true.\n\n")
		for _, h := range gaps {
			fmt.Fprintf(b, "- %s%s\n", h.Term, requiredMark(h))
		}
	}
}

func requiredMark(h tailor.KeywordHit) string {
	if h.Required {
		return " — required"
	}
	return ""
}
