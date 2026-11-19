# best-candidate

Generate a tailored CV and cover letter (PDF) from a job offer, without exaggeration.

You keep one markdown file with everything about yourself. For each job offer, `best-candidate`
selects and rephrases what is relevant, using only facts from that file. Every statement is
checked against the file, and anything that cannot be verified is removed. Requirements your
profile does not show are left out and listed in a report instead of being implied.

## Install

You need [Go](https://go.dev/dl/), [Typst](https://github.com/typst/typst#installation) and an API
key for one model provider.

```sh
git clone https://github.com/BernatRubio/best-candidate
cd best-candidate
go build -o best-candidate ./cmd/best-candidate
export ANTHROPIC_API_KEY=...
```

## Usage

Create your profile once and fill it in:

```sh
./best-candidate init        # writes profile.md from a template
```

Write facts, not marketing, and be generous: the tool picks what is relevant for each job. Say
exactly how much you did ("led", "helped", "contributed to"), because the tool will not upgrade it.
Anything missing from the file can never appear in an output. HTML comments (`<!-- ... -->`) are
ignored, so use them for notes to yourself.

If you list courses, put them under the degree they belong to as `- Area: course, course, course`
lines, and write a class ranking such as `Top 10%` on its own line. The tool picks the most relevant
courses for each job and never names the same course under two degrees: a course shared by two
degrees (for example recognised credits) appears under the one that fits the job best. Spell a
shared course identically in both lists so the repeat can be detected. The template shows the format.

Then generate documents for each offer, given as a file, a URL or stdin:

```sh
./best-candidate generate --job offer.md
./best-candidate generate --job https://example.com/jobs/123
pbpaste | ./best-candidate generate --job -
```

Pages that need a login, such as LinkedIn, cannot be fetched: save the text to a file instead.

| Flag | Meaning |
|---|---|
| `--profile` | profile path (default `profile.md`) |
| `--lang` | `auto` (default) follows the offer's language; or `en`, `es`, `ca`, ... |
| `--out` | output folder (default `out`) |
| `--max-repairs` | correction rounds before unverifiable statements are removed (default 2) |
| `--provider`, `--model`, `--base-url` | model selection, see below |

## Output

Each run writes `out/<company>-<role>-<date>/`:

| File | Content |
|---|---|
| `cv.pdf`, `cover-letter.pdf` | the documents |
| `report.md` | requirements your profile covers and does not show, anything removed, warnings |
| `cv.json`, `letter.json` | the content, editable |
| `cv.typ`, `letter.typ`, `common.typ` | the Typst templates, editable |

After editing the JSON or the templates, rebuild the PDFs with `./best-candidate render out/<folder>`.
The command exits with status 2 if it had to remove statements it could not verify.

## Models

| `--provider` | Credentials | Notes |
|---|---|---|
| `anthropic` (default) | `ANTHROPIC_API_KEY` | default model `claude-opus-5-5` |
| `openai` | `OPENAI_API_KEY` | `--model` required |
| `gemini` | `GEMINI_API_KEY` | `--model` required |
| `ollama` | none | local; `--model` required |
| `openai-compatible` | `BC_API_KEY` | `--base-url` and `--model` required |

`BC_PROVIDER`, `BC_MODEL` and `BC_BASE_URL` set the same options from the environment. If a server
rejects strict JSON-schema output, set `BC_JSON_MODE=object`. Small local models follow the
honesty rules noticeably worse.

## How statements are verified

The model must back every statement with verbatim quotes from your profile. The tool checks that
each quote really exists in the profile, then a second, independent model call fact-checks every
statement. Flagged statements are sent back for correction, and whatever is still unsupported is
removed.

The fact-checker is a model and can miss things, and an inflated profile produces inflated
documents. Always read both documents before sending them.

## Development

```sh
go vet ./... && go test ./...
```

The tests use a scripted fake model and never call a real API. The PDF tests need Typst and are
skipped without it.
