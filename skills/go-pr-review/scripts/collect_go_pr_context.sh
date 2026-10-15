#!/usr/bin/env bash
# Collect a shared Go review context (diff, file read, go vet, go test) into one Markdown report.
# Usage: collect_go_pr_context.sh [review_mode=pr|code] [target=PATH] [base_ref=REF] [head_ref=REF] [out=FILE]
set -uo pipefail

review_mode="pr"
target="."
base_ref="origin/main"
head_ref="HEAD"
out=".go-pr-review/report.md"
max_file_lines="${MAX_FILE_LINES:-400}"
max_diff_lines="${MAX_DIFF_LINES:-1500}"

for arg in "$@"; do
  case "$arg" in
    review_mode=*) review_mode="${arg#*=}" ;;
    target=*)      target="${arg#*=}" ;;
    base_ref=*)    base_ref="${arg#*=}" ;;
    head_ref=*)    head_ref="${arg#*=}" ;;
    out=*)         out="${arg#*=}" ;;
    provider=*|ollama_model=*) ;;  # accepted and ignored so the full invocation can be passed through
    *) echo "Unknown argument: $arg" >&2; exit 2 ;;
  esac
done

case "$review_mode" in pr|code) ;; *) echo "review_mode must be pr or code" >&2; exit 2 ;; esac
[ -e "$target" ] || { echo "target not found: $target" >&2; exit 2; }

mkdir -p "$(dirname "$out")"
: > "$out"
w() { printf '%s\n' "$*" >> "$out"; }
fence() { w '```'"${1:-}"; cat >> "$out"; w '```'; }
truncate_lines() { awk -v max="$1" 'NR<=max {print} NR==max+1 {print "... [truncated after " max " lines]"}'; }

in_git=false
git rev-parse --is-inside-work-tree >/dev/null 2>&1 && in_git=true

w "# Go Review Context"
w ""
w "- generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
w "- review_mode: $review_mode"
w "- target: $target"
$in_git && w "- branch: $(git rev-parse --abbrev-ref HEAD 2>/dev/null) @ $(git rev-parse --short HEAD 2>/dev/null)"
w ""

# ---------- Diff Summary ----------
w "## Diff Summary"
w ""
if [ "$review_mode" = "code" ]; then
  w "_Skipped: review_mode=code (no PR diff requested)._"
elif ! $in_git; then
  w "_Skipped: not inside a git repository._"
else
  diff_args=()
  if git rev-parse --verify --quiet "$base_ref^{commit}" >/dev/null \
     && git rev-parse --verify --quiet "$head_ref^{commit}" >/dev/null; then
    w "- source: \`git diff ${base_ref}...${head_ref}\`"
    diff_args=("${base_ref}...${head_ref}")
  else
    w "- source: working tree \`git diff HEAD\` (refs \`$base_ref\` / \`$head_ref\` not resolvable)"
    if git rev-parse --verify --quiet HEAD >/dev/null; then diff_args=("HEAD"); fi
  fi
  pathspec=()
  [ "$target" != "." ] && pathspec=(-- "$target")
  w ""
  w "### Changed files"
  git diff --stat ${diff_args[@]+"${diff_args[@]}"} ${pathspec[@]+"${pathspec[@]}"} 2>&1 | fence
  w ""
  w "### Patch"
  patch="$(git diff ${diff_args[@]+"${diff_args[@]}"} ${pathspec[@]+"${pathspec[@]}"} 2>&1)"
  if [ -z "$patch" ]; then
    w "_No changes found._"
  else
    printf '%s\n' "$patch" | truncate_lines "$max_diff_lines" | fence diff
  fi
fi
w ""

# ---------- File Read ----------
w "## File Read"
w ""
if [ -f "$target" ]; then
  files=("$target")
else
  files=()
  while IFS= read -r f; do files+=("$f"); done < <(find "$target" -type f -name '*.go' \
    -not -path '*/vendor/*' -not -path '*/.git/*' -not -path '*/.go-pr-review/*' | sort | head -n 50)
fi
if [ "${#files[@]}" -eq 0 ] 2>/dev/null || [ -z "${files[*]:-}" ]; then
  w "_No Go files found under \`$target\`._"
else
  for f in "${files[@]}"; do
    w "### \`$f\`"
    truncate_lines "$max_file_lines" < "$f" | fence go
    w ""
  done
fi
w ""

# ---------- go vet / go test ----------
run_check() {
  local title="$1"; shift
  w "## $title"
  w ""
  if ! command -v go >/dev/null 2>&1; then
    w "_Skipped: \`go\` not found on PATH._"; w ""; return
  fi
  local output rc
  output="$("$@" 2>&1)"; rc=$?
  w "- command: \`$*\`"
  w "- exit_code: $rc ($( [ $rc -eq 0 ] && echo PASS || echo FAIL ))"
  w ""
  if [ -n "$output" ]; then printf '%s\n' "$output" | truncate_lines 600 | fence; else w "_No output._"; fi
  w ""
}
run_check "go vet" go vet ./...
run_check "go test" go test ./...

echo "Report written to $out"
