// Package dotenv loads KEY=value pairs from a .env file into the process
// environment, so API keys can live next to the project instead of being
// exported in every shell.
package dotenv

import (
	"errors"
	"os"
	"regexp"
	"strings"
)

var validKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Load reads path and sets every variable it defines that is not already set
// (and non-empty) in the environment, so a value exported in the shell always
// wins. A missing file is not an error. Values are never logged.
func Load(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for key, value := range Parse(string(data)) {
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

// Parse understands the common .env forms: KEY=value, export KEY=value,
// quoted values ("..." or '...'), full-line comments and trailing " # comment"
// after an unquoted value. Lines it does not understand are skipped.
func Parse(data string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export"); ok && (strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "\t")) {
			line = strings.TrimSpace(rest)
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !validKey.MatchString(key) {
			continue
		}
		out[key] = parseValue(strings.TrimSpace(value))
	}
	return out
}

func parseValue(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
		if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
			return v[1 : 1+end]
		}
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}
