# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
go build -o best-candidate ./cmd/best-candidate   # build (bin/ is git-ignored too)
go vet ./... && go test ./...                     # lint and test
go test ./internal/tailor -run TestPruneRemovesExactlyTheRef -v   # one test
```

Render tests that compile PDFs skip themselves when `typst` is not installed (looked up on PATH, then `~/.cargo/bin/typst`).

Running the tool needs a model API key, read from the shell or from a `.env` file in the working directory (shell wins):

```sh
./best-candidate init                    # writes profile.md from examples/profile.example.md
./best-candidate generate --job offer.md # file, http(s) URL, or - for stdin
./best-candidate render out/<folder>     # rebuild PDFs after editing cv.json/letter.json/*.typ
```

`generate` exits with status 2 when it had to remove statements it could not verify.

Provider settings come from flags or `BC_PROVIDER`, `BC_MODEL`, `BC_BASE_URL`, `BC_API_KEY`. OpenAI-compatible servers also read `BC_JSON_MODE` (`schema` by default, `object`, or `tool`), `BC_MAX_TOKENS` and `BC_REASONING_EFFORT`. OpenRouter's free endpoints (such as `nvidia/nemotron-3-ultra-550b-a55b:free`) ignore `response_format`, so they need `BC_JSON_MODE=tool`. They allow about 50 requests a day, and a full run makes about 7 to 10, so don't burn real runs casually.

## Architecture

The product invariant is honesty: every statement about the candidate must be backed by a verbatim quote from the profile, and whatever cannot be verified is removed rather than kept. Keep that in mind for any change to prompts, checks or rendering.

**Flow** (`cmd/best-candidate/main.go`): `profile.Load` → `joboffer.Load` → `llm.FromEnv` (+ `WithProgress`) → `tailor.Run` → `render.Build`.

**`internal/tailor`**, the core, split in two:
- `pipeline.go` holds only the model protocol. It makes three kinds of call: `analyze` (offer → `JobAnalysis`), `tailorDocs` (writer → CV and letter) and `verify` (fact-checker). The loop is tailor → verify → repair, for up to `MaxRepairs` rounds. `verify` re-asks once about claims the fact-checker skipped, because some models stop part way through long lists.
- `review.go` (`draftReview`) holds everything the program checks and fixes itself, and decides their order:
  - `Prepare` runs after every tailoring call and sorts degrees before any claim ref is taken.
  - `Review` combines quote grounding, rule checks and the fact-checker's verdicts into `findings` (issues, repeated courses, keyword gaps).
  - `GiveUp` drops the issues that persist.
  - `Finalize` grounds identity, rankings and links, cleans titles, and fills empty Skills/Languages from the profile.

  ATS is computed last, in `Run`, and refreshed by `render.Build` after the one-page cuts (`Result.RefreshATS`).
- `claims.go`: `Documents.claims()` is the single walker over the documents. `ClaimRef` is positional and renders the ids the model and `report.md` see (`cv.experience[0].bullets[1]`). Refs are invalid after any change to the documents, which is why sorting happens in `Prepare`.
- `schema.go`: the JSON types. The schemas sent to the model are derived from them with `llm.SchemaFor`, so `jsonschema` tag descriptions are part of the prompt. The writer's schema is `draft` (CV and letter only); coverage comes from the fact-checker alone.
- `prompts/*.md` are embedded with `go:embed`. Tests pin key phrases of the prompts (`TestATSPromptsAndSchema`, `TestPromptsCoverKeyBehaviours`, `TestPromptMatchesSchema`), so rewording a prompt may mean updating those lists.

**`internal/profile`**: `Parse` turns the markdown into a `Profile`. It strips HTML comments (they are notes, never evidence), recognises section headings in English, Spanish and Catalan, and owns token normalisation (`Normalize`) and quote lookup (`Has`). `tailor` must ask the `Profile` rather than re-parse the text.

**`internal/llm`**: `Provider` is the seam (`GenerateJSON(ctx, Request) → json.RawMessage`). It has three adapters: Anthropic (default `claude-opus-5-5`, SDK retries), Gemini, and `OpenAICompat` (OpenAI, Ollama, any compatible server). `New` composes the adapters with `WithRetry`; `Fake` is the scripted test double. `ExtractJSON` tolerates code fences, surrounding prose and leaked `<think>` blocks.

**`internal/render`**: writes `cv.json`, `letter.json` and the Typst templates (`templates/`, embedded) into `out/<company>-<role>-<date>/`, then compiles with the `typst` CLI. `Fit` keeps the CV to one page by cutting the least relevant item from the end of each list, with the page counter passed in so it can be tested without typst. `report.go` writes `report.md`.

## Testing conventions

`tailor` pipeline tests script `llm.NewFake` with one handler per model call, in call order (analyze, tailor, verify, [re-review], ...). Changing how many calls `Run` makes breaks them. Prefer testing checks directly through `draftReview` (`review_test.go`), which needs no fake.

Domain terms are defined in `GLOSSARY.md`.
