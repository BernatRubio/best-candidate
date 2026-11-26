# best-candidate

Generate a tailored CV and cover letter (PDF) from a job offer, without exaggeration.

You keep one markdown file with everything about yourself. For each job offer, `best-candidate`
selects and rephrases what is relevant, using only facts from that file. The model is told to back
every statement with a verbatim quote from your profile. Nothing is verified automatically yet.

## Install

You need [Go](https://go.dev/dl/), [Typst](https://github.com/typst/typst#installation) and an API
key for an OpenAI-compatible server (OpenRouter by default).

```sh
git clone https://github.com/BernatRubio/best-candidate
cd best-candidate
go build -o best-candidate ./cmd/best-candidate
export BC_API_KEY=sk-or-...
```

Instead of exporting keys in every shell, you can put them in a `.env` file in the folder you run the
command from (for example `BC_API_KEY=...`). It is git-ignored, and variables already set in the shell
take precedence.

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
lines, and write a class ranking such as `Top 10%` on its own line. The template shows the format.

Then generate documents for each offer, given as a markdown file or a URL:

```sh
./best-candidate generate --model nvidia/nemotron-3-ultra-550b-a55b:free offer.md
./best-candidate generate --model <model> https://example.com/jobs/123
```

Pages that need a login, such as LinkedIn, cannot be fetched: save the text to a file instead.

| Flag | Meaning |
|---|---|
| `--profile` | profile path (default `profile.md`) |
| `--lang` | `auto` (default) follows the offer's language; or `en`, `es`, `ca`, ... |
| `--out` | output folder (default `out`) |
| `--model` | model id, required (env `BC_MODEL`) |
| `--base-url` | OpenAI-compatible API URL (default `https://openrouter.ai/api/v1`, env `BC_BASE_URL`) |

## Output

Each run writes `out/<company>-<role>-<date>/`:

| File | Content |
|---|---|
| `cv.pdf`, `cover-letter.pdf` | the documents |
| `report.md` | the offer, the language and any warnings |
| `cv.json`, `letter.json` | the content, editable |
| `cv.typ`, `letter.typ`, `common.typ` | the Typst templates, editable |

The CV is always kept to one page: the font size and spacing adapt to the content, and if that is not
enough the least relevant content is removed from the end of each list (every cut is listed in
`report.md`). A cover letter longer than one page is reported but not cut.

After editing the JSON or the templates, rebuild the PDFs with `./best-candidate render out/<folder>`.

## Model

The tool talks to any OpenAI-compatible `/chat/completions` server, OpenRouter by default. The API
key comes from `BC_API_KEY`. The JSON output is requested as a forced tool call, so the model must
support tools. Each run makes one model call.

The model can still exaggerate: always read both documents before sending them.

## Development

```sh
go vet ./... && go test ./...
```

The tests use a scripted fake model and never call a real API. The PDF tests need Typst and are
skipped without it.
