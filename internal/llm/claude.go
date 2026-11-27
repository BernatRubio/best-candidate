package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ClaudeCLI runs the Claude Code CLI in print mode, so calls use the logged-in
// Claude plan instead of an API key.
type ClaudeCLI struct {
	Model string // optional: the CLI's default model when empty
}

func (c *ClaudeCLI) Name() string {
	if c.Model == "" {
		return "claude"
	}
	return "claude " + c.Model
}

// GenerateJSON runs one `claude -p` call with every tool, MCP server and skill
// disabled. The CLI validates the answer against req.Schema.
func (c *ClaudeCLI) GenerateJSON(ctx context.Context, req Request) (json.RawMessage, error) {
	args := []string{"-p", "--output-format", "json", "--no-session-persistence",
		"--safe-mode", "--tools", "", "--strict-mcp-config", "--disable-slash-commands",
		"--system-prompt", req.System, "--json-schema", string(req.Schema)}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(req.User)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, runErr := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var res struct {
		IsError          bool            `json:"is_error"`
		Result           string          `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		if runErr != nil {
			return nil, fmt.Errorf("claude: %w: %s", runErr, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("decoding claude output: %w", err)
	}
	if res.IsError || runErr != nil {
		return nil, fmt.Errorf("claude: %s", res.Result)
	}
	if len(res.StructuredOutput) > 0 && string(res.StructuredOutput) != "null" {
		return res.StructuredOutput, nil
	}
	return ExtractJSON(res.Result)
}
