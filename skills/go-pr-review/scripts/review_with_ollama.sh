#!/usr/bin/env bash
# Summarize a Go review report with a local Ollama model.
# Usage: review_with_ollama.sh [report=FILE] [model=NAME] [out=FILE]
set -uo pipefail

report=".go-pr-review/report.md"
model="qwen2.5-coder:7b"
out=".go-pr-review/ollama_review.md"
max_chars="${OLLAMA_MAX_CHARS:-60000}"

for arg in "$@"; do
  case "$arg" in
    report=*) report="${arg#*=}" ;;
    model=*|ollama_model=*) model="${arg#*=}" ;;
    out=*) out="${arg#*=}" ;;
    *) echo "Unknown argument: $arg" >&2; exit 2 ;;
  esac
done

if ! command -v ollama >/dev/null 2>&1; then
  echo "ERROR: ollama CLI not found. Install it from https://ollama.com, then run: ollama pull $model" >&2
  exit 3
fi
if ! ollama list 2>/dev/null | awk 'NR>1 {print $1}' | grep -qx -e "$model" -e "$model:latest"; then
  echo "ERROR: model '$model' is not pulled (or the ollama server is not running). Run: ollama pull $model" >&2
  exit 4
fi
[ -f "$report" ] || { echo "ERROR: report not found: $report (run collect_go_pr_context.sh first)" >&2; exit 2; }

context="$(head -c "$max_chars" "$report")"
[ "$(wc -c < "$report")" -gt "$max_chars" ] && context="$context
... [report truncated to $max_chars characters]"

prompt="You are a senior Go reviewer. Review the context below, which was collected from a Go repository.
Be specific and cite file names and line content. Do not invent code that is not shown.

Respond in Markdown with exactly these sections, in this order:
## Diff Summary
What changed, by file, in a few lines (say 'No diff' if the diff was skipped or empty).
## File Read
Notable issues in the code: bugs, error handling, concurrency, resource leaks, API misuse, readability.
## go vet
PASS or FAIL, and explain each finding.
## go test
PASS or FAIL, failing tests and their likely cause.
## Next Action
A short prioritized list of what to fix or do before merging.

--- BEGIN CONTEXT ---
$context
--- END CONTEXT ---"

mkdir -p "$(dirname "$out")"
if ! printf '%s' "$prompt" | ollama run "$model" > "$out"; then
  echo "ERROR: ollama run failed for model '$model'" >&2
  exit 5
fi
cat "$out"
