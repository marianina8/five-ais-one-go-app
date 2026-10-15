---
name: go-pr-provider
description: "Route Go PR or code validation by provider flag using one shared check pipeline (git diff, file read, go vet, go test), then summarize with Claude or local Ollama. Use when the user asks to review Go changes, a Go PR, a direct commit, or a Go file/folder, especially with provider=claude or provider=ollama."
compatibility: "Requires git and the go toolchain; provider=ollama also requires the ollama CLI with a pulled model."
metadata:
  argument-hint: "provider=claude|ollama review_mode=pr|code target=path/to/file-or-dir base_ref=origin/main head_ref=HEAD ollama_model=qwen2.5-coder:7b"
---

# Go PR Provider Router

## When to Use
- You want one command-like workflow for Go PR checks.
- You want to review direct commits or any code even without an open PR.
- You want to choose the review provider with a flag:
  - `provider=claude` summarizes with Claude in chat.
  - `provider=ollama` summarizes with a local `ollama` model.

## Arguments
- `provider` (required): `claude` or `ollama`
- `review_mode` (optional):
  - `pr` (default): produce the diff from refs when available
  - `code`: review target code even without a PR diff
- `target` (optional): file or directory to inspect (default: repo root, `.`)
- `base_ref` (optional, `review_mode=pr`): default `origin/main`
- `head_ref` (optional, `review_mode=pr`): default `HEAD`
- `ollama_model` (optional, only for `provider=ollama`): default `qwen2.5-coder:7b`

If `provider` is missing or not one of `claude`/`ollama`, ask the user which one to use before running anything.

## Procedure
1. **Parse arguments** from the invocation (`key=value` pairs). Apply the defaults above.
2. **Collect context (identical for both providers).** From the root of the Go module/repo, run:
   ```bash
   bash <skill-dir>/scripts/collect_go_pr_context.sh \
     review_mode=<mode> target=<target> base_ref=<base> head_ref=<head> \
     out=.go-pr-review/report.md
   ```
   The script writes a Markdown report with these sections:
   - **Diff Summary** — in `pr` mode, `git diff <base_ref>...<head_ref>` when both refs resolve; otherwise falls back to the working-tree `git diff` (staged + unstaged). In `code` mode, the diff is skipped.
   - **File Read** — contents of `target` (Go files only when a directory; large files are truncated).
   - **go vet** — output and exit code of `go vet ./...`
   - **go test** — output and exit code of `go test ./...`

   The script never aborts on check failures; it records exit codes so the review can explain them.
3. **Route by provider:**
   - `provider=claude`: read `.go-pr-review/report.md` and write the review yourself in chat.
   - `provider=ollama`: run
     ```bash
     bash <skill-dir>/scripts/review_with_ollama.sh \
       report=.go-pr-review/report.md model=<ollama_model> \
       out=.go-pr-review/ollama_review.md
     ```
     then read `.go-pr-review/ollama_review.md` and return it, lightly cleaned up into the section order below. If `ollama` is missing or the model is not pulled, say so and show the fix (`ollama pull <model>`); offer to finish with `provider=claude` instead.
4. **Return results in this section order:**
   - `Diff Summary` — what changed, by file, in a few lines
   - `File Read` — notable issues in the target code (bugs, error handling, concurrency, API misuse)
   - `go vet` — pass/fail and each finding explained
   - `go test` — pass/fail, failing tests and likely cause
   - `Next Action` — a short prioritized list of what to fix or do before merging

## Notes
- The check execution path is identical for both providers because both must run `scripts/collect_go_pr_context.sh`; only the summarizer differs.
- Supports PR review and non-PR code review using the same collector.
- `provider=ollama` requires the local `ollama` CLI and a pulled model. Recommended first run:
  `ollama pull qwen2.5-coder:7b`
- Add `.go-pr-review/` to `.gitignore` so reports are never committed.

## Examples
- PR-style review from branch refs:
  `provider=claude review_mode=pr base_ref=origin/main head_ref=HEAD`
- Review direct-commit changes (working-tree fallback):
  `provider=claude review_mode=pr`
- Review any code file or folder without PR context:
  `provider=ollama review_mode=code target=internal/service`
