// Package profile loads the user's master profile: a free-form markdown file
// holding everything the tool is allowed to know about them.
package profile

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	blankRuns   = regexp.MustCompile(`\n{3,}`)
)

// Load reads and parses the profile file. HTML comments are removed first:
// they are notes for the user (TODOs, tips) and must not reach the model or be
// accepted as evidence for a claim. Everything else is kept verbatim.
func Load(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile: %w", err)
	}
	p := Parse(string(data))
	if p.Text() == "" {
		return nil, fmt.Errorf("profile %s is empty (HTML comments are ignored)", path)
	}
	return p, nil
}

// Clean strips HTML comments and tidies the blank lines they leave behind.
func Clean(s string) string {
	s = htmlComment.ReplaceAllString(s, "")
	s = blankRuns.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
