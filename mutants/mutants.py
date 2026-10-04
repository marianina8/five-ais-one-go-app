#!/usr/bin/env python3
"""Mutation check for the hidden tests.

Each mutant is the reference implementation with one realistic bug planted in it.
The hidden tests must fail every mutant, in the test written to catch that bug.
If a mutant survives, the tests have a hole.

    python3 mutants/mutants.py                   # every mutant
    python3 mutants/mutants.py nil_list ...      # just these
"""
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
from dataclasses import dataclass

ROOT = pathlib.Path(__file__).resolve().parent.parent
REFERENCE = ROOT / "reference"
ACCEPTANCE = ROOT / "acceptance"


@dataclass
class Mutant:
    bug: str                              # what a model might get wrong
    edits: list[tuple[str, str, str]]     # (file, old text, new text)
    caught_by: str                        # the hidden test that must fail


MUTANTS = {
    "visit_without_lock": Mutant(
        "Counts visits without holding the lock (lost updates, data race)",
        [("store.go",
          "\ts.mu.Lock()\n\tdefer s.mu.Unlock()\n\n\tlink, found := s.links[code]\n\tif !found {\n\t\treturn \"\", false, nil\n\t}\n\tlink.Visits++\n",
          "\ts.mu.Lock()\n\tlink, found := s.links[code]\n\ts.mu.Unlock()\n\tif !found {\n\t\treturn \"\", false, nil\n\t}\n"
          "\tvisits := link.Visits\n\ttime.Sleep(time.Millisecond)\n\tlink.Visits = visits + 1\n\ts.mu.Lock()\n\tdefer s.mu.Unlock()\n")],
        "TestConcurrency_ParallelVisitsExact"),
    "write_in_place": Mutant(
        "Rewrites the data file in place instead of writing a temp file and renaming it",
        [("store.go",
          "\treturn writeFileAtomic(s.path, data)",
          "\treturn writeInPlace(s.path, data)\n}\n\n"
          "func writeInPlace(path string, data []byte) error {\n"
          "\tfile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)\n"
          "\tif err != nil {\n\t\treturn err\n\t}\n\tdefer file.Close()\n"
          "\tfor i := 0; i < len(data); i += 4096 {\n"
          "\t\tif _, err := file.Write(data[i:min(i+4096, len(data))]); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n"
          "\treturn nil")],
        "TestPersistence_KillDuringWrites"),
    "list_without_auth": Mutant(
        "Forgets the admin check on GET /api/links (anyone can list every link)",
        [("api.go", "api.requireAdmin(api.listLinks)", "api.listLinks")],
        "TestAdmin_ListNeedsToken/no_header"),
    "token_contains": Mutant(
        "Checks the token with strings.Contains instead of an exact match",
        [("api.go",
          "token, isBearer := strings.CutPrefix(r.Header.Get(\"Authorization\"), \"Bearer \")\n"
          "\t\tif !isBearer || subtle.ConstantTimeCompare([]byte(token), []byte(api.adminToken)) != 1 {",
          "_ = subtle.ConstantTimeCompare\n"
          "\t\tif !strings.Contains(r.Header.Get(\"Authorization\"), api.adminToken) {")],
        "TestAdmin_ListNeedsToken/token_longer"),
    "nil_list": Mutant(
        "Sends an empty list as null instead of []",
        [("store.go", "snapshot := make([]Link, 0, len(s.order))", "var snapshot []Link")],
        "TestAdmin_ListEmptyIsArray"),
    "any_scheme": Mutant(
        "Only checks that the URL has a host, so ftp:// gets through",
        [("validate.go",
          "return (u.Scheme == \"http\" || u.Scheme == \"https\") && u.Hostname() != \"\"",
          "return u.Host != \"\"")],
        "TestValidation_BadURLs/ftp_scheme"),
    "no_body_limit": Mutant(
        "No limit on the request body",
        [("api.go", "json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))", "json.NewDecoder(r.Body)")],
        "TestErrors_BodyTooLarge413"),
    "unknown_fields_ok": Mutant(
        "Accepts unknown JSON fields",
        [("api.go", "\tdecoder.DisallowUnknownFields()\n", "")],
        "TestValidation_UnknownField"),
    "alias_check_then_act": Mutant(
        "Checks the alias under one lock and inserts it under another (race)",
        [("store.go",
          "\t} else if _, exists := s.links[code]; exists {\n\t\treturn Link{}, ErrAliasTaken\n\t}",
          "\t} else if _, exists := s.links[code]; exists {\n\t\treturn Link{}, ErrAliasTaken\n\t} else {\n"
          "\t\ts.mu.Unlock()\n\t\ttime.Sleep(2 * time.Millisecond)\n\t\ts.mu.Lock()\n\t}")],
        "TestConcurrency_SameAliasOnceOnly"),
    "save_later": Mutant(
        "Saves in the background 2 s later instead of before answering",
        [("store.go",
          "\ts.order = append(slices.Clip(s.order), code)\n\tif err := s.saveLocked(); err != nil {",
          "\ts.order = append(slices.Clip(s.order), code)\n\tif err := s.saveSoon(); err != nil {"),
         ("store.go",
          "func (s *Store) snapshotLocked() []Link {",
          "func (s *Store) saveSoon() error {\n\tgo func() {\n\t\ttime.Sleep(2 * time.Second)\n"
          "\t\ts.mu.Lock()\n\t\tdefer s.mu.Unlock()\n\t\t_ = s.saveLocked()\n\t}()\n\treturn nil\n}\n\n"
          "func (s *Store) snapshotLocked() []Link {")],
        "TestPersistence_KillAndRecoverWithoutRestartGrace"),
    "visits_in_memory": Mutant(
        "Keeps visit counts in memory and only saves them with the next other change",
        [("store.go",
          "\tlink.Visits++\n\tif err := s.saveLocked(); err != nil {\n\t\tlink.Visits--\n\t\treturn \"\", true, err\n\t}\n",
          "\tlink.Visits++\n")],
        "TestPersistence_KillAndRecoverWithoutRestartGrace"),
    "plain_text_errors": Mutant(
        "Sends errors as plain text (http.Error) instead of JSON",
        [("api.go",
          "\twriteJSON(w, status, map[string]string{\"error\": message})",
          "\thttp.Error(w, message, status)")],
        "TestErrors_JSONErrorFormat"),
    "runs_without_token": Mutant(
        "Starts without ADMIN_TOKEN, so an empty token is accepted",
        [("main.go",
          "\tif adminToken == \"\" {\n\t\tfmt.Fprintln(os.Stderr, \"shortener: ADMIN_TOKEN must be set\")\n\t\tos.Exit(1)\n\t}",
          "\t_ = fmt.Sprint")],
        "TestAdmin_StartWithoutTokenFails"),
    "short_codes": Mutant(
        "Generates 6-character codes",
        [("store.go", "codeLength   = 7", "codeLength   = 6")],
        "TestCore_CreateGeneratedCode"),
    "reserved_api_allowed": Mutant(
        "Lets someone take the alias \"api\"",
        [("validate.go", "aliasPattern.MatchString(alias) && alias != reservedAlias", "aliasPattern.MatchString(alias)")],
        "TestValidation_BadAliases/reserved_api"),
}


def build_mutant(mutant: Mutant, folder: pathlib.Path) -> pathlib.Path:
    """Copies the reference into folder, applies the mutant's edits and builds it."""
    shutil.copytree(REFERENCE, folder, dirs_exist_ok=True)
    for name, old, new in mutant.edits:
        path = folder / name
        source = path.read_text()
        if old not in source:
            sys.exit(f"edit for {name} no longer matches the reference: {old[:70]!r}")
        path.write_text(source.replace(old, new, 1))
    binary = folder / "shortener"
    build = subprocess.run(["go", "build", "-o", str(binary), "."], cwd=folder, capture_output=True, text=True)
    if build.returncode:
        sys.exit(f"mutant does not build:\n{build.stderr}")
    return binary


def run_hidden_tests(binary: pathlib.Path) -> dict[str, str]:
    """Runs the hidden tests against binary and returns {leaf test: pass|fail}."""
    env = dict(os.environ, SHORTENER_BIN=str(binary))
    try:
        output = subprocess.run(["go", "test", "-count=1", "-json", "./..."], cwd=ACCEPTANCE,
                                env=env, capture_output=True, text=True, timeout=900).stdout
    except subprocess.TimeoutExpired:
        return {}  # nothing reported: the target test counts as "not run", so the mutant is MISSED
    results = {}
    for line in output.splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            continue
        if event.get("Test") and event["Action"] in ("pass", "fail"):
            results[event["Test"]] = event["Action"]
    parents = {test.rsplit("/", 1)[0] for test in results if "/" in test}
    return {test: action for test, action in results.items() if test not in parents}


def main() -> None:
    chosen = sys.argv[1:] or list(MUTANTS)
    unknown = [name for name in chosen if name not in MUTANTS]
    if unknown:
        sys.exit(f"unknown mutant(s): {', '.join(unknown)}; choose from: {', '.join(MUTANTS)}")
    missed = 0
    for name in chosen:
        mutant = MUTANTS[name]
        with tempfile.TemporaryDirectory() as folder:
            results = run_hidden_tests(build_mutant(mutant, pathlib.Path(folder)))
        failed = sorted(test for test, action in results.items() if action == "fail")
        caught = results.get(mutant.caught_by) == "fail"
        missed += not caught
        print(f"{'CAUGHT ' if caught else 'MISSED!'} {name:22} {mutant.bug}")
        print(f"        {mutant.caught_by}: {results.get(mutant.caught_by, 'not run')}; "
              f"{len(failed)}/{len(results)} tests fail")
        for test in failed:
            print(f"          - {test}")
    print(f"\n{missed} mutant(s) not caught by their target test")
    sys.exit(1 if missed else 0)


if __name__ == "__main__":
    main()
