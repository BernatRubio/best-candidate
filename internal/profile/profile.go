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

// Load reads the profile file and returns its content. HTML comments are
// removed first: they are notes for the user (TODOs, tips) and must not reach
// the model or be accepted as evidence for a claim. Everything else is kept
// verbatim; the profile is not parsed.
func Load(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading profile: %w", err)
	}
	text := Clean(string(data))
	if text == "" {
		return "", fmt.Errorf("profile %s is empty (HTML comments are ignored)", path)
	}
	return text, nil
}

// Clean strips HTML comments and tidies the blank lines they leave behind.
func Clean(s string) string {
	s = htmlComment.ReplaceAllString(s, "")
	s = blankRuns.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
