# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

The project was deliberately cut back to a barebone version so the user can grow it by hand. Keep changes small and don't reintroduce removed features (other providers, fact-checking, repair loops, ATS scoring) unless asked.

## Commands

```sh
go build -o bin/best-candidate ./cmd/best-candidate   # build
go vet ./... && go test ./...                     # lint and test
go test ./internal/tailor -run TestRunLanguage -v # one test
```

Render tests that compile PDFs skip themselves when `typst` is not installed (looked up on PATH, then `~/.cargo/bin/typst`).

Running the tool needs the `claude` CLI on PATH, logged in (calls use the Claude plan, no API key). Env vars can also come from a `.env` file in the working directory (shell wins):

```sh
./best-candidate init                                 # writes profile.md from examples/profile.example.md
./best-candidate generate offer.md                    # markdown file or http(s) URL
./best-candidate render out/<folder>                  # rebuild PDFs after editing cv.json/letter.json/*.typ
```

`--model`/`BC_MODEL` is optional (`sonnet`, `opus`, ...; the CLI's default otherwise). A run makes one call.

## Architecture

Goal: every statement about the candidate is backed by a verbatim quote from the profile (`sources` on each `Claim`). The prompt asks for this; nothing verifies it yet.

**Flow** (`cmd/best-candidate/main.go`): `profile.Load` → `joboffer.Load` → `llm.FromEnv` (+ `WithProgress`) → `tailor.Run` → `render.Build`.

- **`internal/profile`**: loads the markdown profile and strips HTML comments (they are notes, never evidence).
- **`internal/joboffer`**: reads the offer from a file or fetches a URL (readability + html-to-markdown).
- **`internal/llm`**: `Provider` is the seam (`GenerateJSON(ctx, Request) → json.RawMessage`). The only adapter is `ClaudeCLI`; it runs `claude -p --json-schema` with all tools, MCP servers and skills disabled, and reads `structured_output`. `Fake` is the scripted test double. `SchemaFor` derives the JSON schema from Go types, so `jsonschema` tag descriptions are part of the prompt. `ExtractJSON` tolerates code fences, prose and leaked `<think>` blocks.
- **`internal/tailor`**: one model call. `Run` sends `prompts/tailor.md` (embedded) with the profile, offer and language, and decodes `output` (company, role, language, CV, letter). `schema.go` holds the JSON types.
- **`internal/render`**: writes `cv.json`, `letter.json` and the Typst templates (`templates/`, embedded) into `out/<company>-<role>-<date>/` and compiles them with the `typst` CLI. `Fit` keeps the CV to one page by cutting the last item of each list, with the page counter passed in so it can be tested without typst. `report.go` writes `report.md`.

## Testing conventions

`tailor` tests script `llm.NewFake` with one handler per model call (`Run` makes exactly one). `llm` tests put a fake `claude` script on PATH.
