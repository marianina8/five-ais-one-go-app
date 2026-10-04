# 5 AIs Built the Same Go App. My Agent Graded Them.

Five AI models got the same one-page spec and built the same small Go service, a URL shortener, with no human help. This repository has everything used to judge them, so you can check the scoring yourself:

- the spec, word for word what every model received
- the hidden tests the models never saw
- the scorer
- each model's code, logs and scores (in `results/`, once the runs are done)

From the YouTube series **Build It in Go**, episode 2. Episode 1 built the review agent that grades the code here: [marianina8/pr-review-agent](https://github.com/marianina8/pr-review-agent).

## Results

> **The runs haven't happened yet.** Everything below was written and checked *before* any model ran. The scorecard, each model's code and every finding with its verdict will appear in [`results/`](results/).

| Contestant | Provider |
|---|---|
| Claude Sonnet 5.5 | Anthropic API |
| Claude Opus 5.5 | Anthropic API |
| GPT-6.1 Sol | OpenAI API |
| Gemini 3.1 Pro (preview) | Gemini API |
| DeepSeek V3.2 | Amazon Bedrock |

## The rules

- **Same everything.** Every model gets the same spec, the same tools (list, read and write files; `go build`, `go test`, `go vet`), the same step and time limits, an empty folder and no network.
- **Hidden tests.** 55 tests written before the contest. They start the program and talk to it over HTTP, so they don't depend on how a model organized its code.
- **Two runs per model.** The scorecard shows the average; a difference of a few points is noise.
- **Blind review.** Before the human review, the projects are renamed to random letters.
- **Claude grades Claude?** The review agent runs on Claude, and Claude is a contestant. So the agent's findings only count once a human confirms them, and 80 of the 100 points come from tests and tools, not from any AI judge.

## Scoring (100 points)

| Category | Points | How |
|---|---|---|
| Does it work? | 40 | Hidden tests, by category: Core 8, Validation 8, Admin & auth 7, Errors 3, Persistence 7, Concurrency 7. Each category scores the share of its tests passed. A project that doesn't build scores 0 in every measured category. |
| Bugs and security | 20 | Starts at 20. Review-agent findings a human confirms cost −4 (high), −2 (medium) or −1 (low). Confirmed go vet, staticcheck, gosec, errcheck and race-detector findings cost −2 each. The same issue found twice counts once. Floor 0. |
| Readable Go | 10 | Style issues (staticcheck S1/ST/QF, ineffassign, unused, misspell, gofmt) plus max(0, highest cyclomatic complexity − 10). Lower is better; models are ranked 10/8/6/4/2, and ties share the better rank. |
| Its own tests | 10 | 0 if its tests fail. Otherwise coverage: 0% = 0, 80% or more = 10, linear between. |
| Would I merge it? | 20 | Blind review: would I approve it as a pull request (0–10), and could a teammate understand it in five minutes (0–10). |
| Cost and speed | shown, not scored | Model calls, tokens, dollars, wall-clock time. |

Tie-break: fewer confirmed high-severity bugs (the review agent rates severity; for a confirmed tool finding, the person confirming it sets one), then lower cost.

## Can the tests be trusted?

Two checks run on every push ([verify-tests workflow](.github/workflows/verify-tests.yml)):

1. **A reference implementation passes all 55 tests.** It's in [`reference/`](reference/), written by hand to the same spec. If the tests demanded something the spec doesn't say, it would fail.
2. **15 planted bugs are all caught.** [`mutants/`](mutants/) makes 15 copies of the reference, each with one realistic mistake: a missing admin check, a token compared with `Contains`, an empty list sent as `null`, an unlocked visit counter, a data file rewritten in place, and more. Each must fail the test written to catch it.

Run them yourself (Go 1.24+, Python 3):

```bash
go -C reference build -o /tmp/shortener .
SHORTENER_BIN=/tmp/shortener go -C acceptance test -v ./...   # 55/55 pass
python3 mutants/mutants.py                                     # 15/15 bugs caught
```

## What's here

| Folder | What it is |
|---|---|
| [`spec/SPEC.md`](spec/SPEC.md) | The prompt every model received |
| [`acceptance/`](acceptance/) | The 55 hidden black-box tests |
| [`reference/`](reference/) | A hand-written implementation that proves the tests are correct (never shown to the models) |
| [`mutants/`](mutants/) | The planted-bug check |
| [`score/`](score/) | The scorer, in Go: `measure` (tests, race detector, linters, coverage), `blind` (renames projects for review), `tally` (the scorecard) |
| [`results/`](results/) | Each model's code, logs, findings and scores, after the runs |

## Scoring a project yourself

```bash
go -C score build -o /tmp/score .
/tmp/score measure --project path/to/project --out results/my-model/run1
#   -> measure.json (what the machine measured) and findings.json (each finding waits for a verdict)
/tmp/score blind --runs runs --results results --review blind-review
#   -> blind-review/A, B, ... and the sealed letter mapping in results/blind/map.json
/tmp/score tally --results results
#   -> results/scorecard.md and scorecard.json; marked provisional while a verdict or review is missing
```

The scorer runs the project's code with a minimal environment (no API keys, no cloud credentials, no module downloads), on a copy of its folder. Only run code you trust. It needs Go 1.24+ and golangci-lint v2 on your PATH.
