package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	for in, want := range map[string]string{
		`{"a":1}`:                              `{"a":1}`,
		"```json\n{\"a\":1}\n```":              `{"a":1}`,
		"```\n{\"a\":1}\n```":                  `{"a":1}`,
		"Here you go: {\"a\":1} Hope it helps": `{"a":1}`,
	} {
		got, err := ExtractJSON(in)
		if err != nil || string(got) != want {
			t.Errorf("ExtractJSON(%q) = %s, %v", in, got, err)
		}
	}
	if _, err := ExtractJSON("no json here"); err == nil {
		t.Error("expected error")
	}
}

func TestExtractJSONSkipsThinking(t *testing.T) {
	out, err := ExtractJSON("<think>the schema wants {x}</think>```json\n{\"a\":1}\n```")
	if err != nil || string(out) != `{"a":1}` {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

func TestClaudeCLI(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" > " + dir + "/args\ncat > " + dir + "/stdin\n" +
		`echo '{"is_error":false,"result":"","structured_output":{"ok":true}}'` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	c := &ClaudeCLI{Model: "sonnet", Effort: "high"}
	out, err := c.GenerateJSON(context.Background(), Request{System: "be honest", User: "hello", Schema: json.RawMessage(`{"type":"object"}`)})
	if err != nil || string(out) != `{"ok":true}` {
		t.Fatalf("out=%s err=%v", out, err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	for _, want := range []string{"--tools  --strict-mcp-config --disable-slash-commands", "--system-prompt be honest", `--json-schema {"type":"object"}`, "--model sonnet", "--effort high"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("args %q miss %q", args, want)
		}
	}
	if stdin, _ := os.ReadFile(filepath.Join(dir, "stdin")); string(stdin) != "hello" {
		t.Errorf("stdin = %q", stdin)
	}
}
