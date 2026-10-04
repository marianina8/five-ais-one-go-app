#!/usr/bin/env python3
"""Mutation check for the hidden tests.

Each mutant is the reference implementation with one realistic bug planted in it.
The hidden tests must fail every mutant, and must fail it in the test meant to catch that bug.
If a mutant survives, the tests have a hole.

    python3 mutants/mutants.py            # from the ep02 folder
"""
import json, os, pathlib, shutil, subprocess, sys, tempfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
REF = (ROOT / "reference" / "main.go").read_text()

# name: (what the bug is, [(old, new), ...], test that must catch it)
MUTANTS = {
    "visit_without_lock": (
        "Visit counts without the mutex (lost updates, data race)",
        [("func (s *Store) Visit(code string) (string, bool, error) {\n\ts.mu.Lock()\n\tdefer s.mu.Unlock()\n\tl, ok := s.links[code]",
          "func (s *Store) Visit(code string) (string, bool, error) {\n\ts.mu.Lock()\n\tl, ok := s.links[code]\n\ts.mu.Unlock()"),
         ("\tl.Visits++\n\tif err := s.saveLocked(); err != nil {",
          "\tv := l.Visits\n\ttime.Sleep(time.Millisecond)\n\tl.Visits = v + 1\n\ts.mu.Lock()\n\tdefer s.mu.Unlock()\n\tif err := s.saveLocked(); err != nil {")],
        "TestConcurrency_ParallelVisitsExact"),
    "write_in_place": (
        "Rewrites the data file in place instead of temp file + rename",
        [("\ttmp, err := os.CreateTemp(filepath.Dir(s.path), \".links-*.tmp\")",
          "\treturn writeInPlace(s.path, data)\n\ttmp, err := os.CreateTemp(filepath.Dir(s.path), \".links-*.tmp\")"),
         ("func (s *Store) listLocked() []Link {",
          "func writeInPlace(path string, data []byte) error {\n\tf, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer f.Close()\n\tfor i := 0; i < len(data); i += 4096 {\n\t\tif _, err := f.Write(data[i:min(i+4096, len(data))]); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\treturn nil\n}\n\nfunc (s *Store) listLocked() []Link {")],
        "TestPersistence_KillDuringWrites"),
    "list_without_auth": (
        "Forgot the admin check on GET /api/links (leaks every link)",
        [('mux.HandleFunc("GET /api/links", srv.admin(srv.list))', 'mux.HandleFunc("GET /api/links", srv.list)')],
        "TestAdmin_ListNeedsToken/no_header"),
    "token_contains": (
        "Checks the token with strings.Contains instead of an exact match",
        [('got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")\n\t\tif !ok || subtle.ConstantTimeCompare([]byte(got), []byte(srv.token)) != 1 {',
          'got := r.Header.Get("Authorization")\n\t\t_ = subtle.ConstantTimeCompare\n\t\tif !strings.Contains(got, srv.token) {')],
        "TestAdmin_ListNeedsToken/token_longer"),
    "nil_list": (
        "Empty list encodes as null instead of []",
        [("out := make([]Link, 0, len(s.seq))", "var out []Link")],
        "TestAdmin_ListEmptyIsArray"),
    "any_scheme": (
        "Only checks that the URL parses and has a host (ftp:// gets through)",
        [('return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""', 'return u.Host != ""')],
        "TestValidation_BadURLs/ftp_scheme"),
    "no_body_limit": (
        "No request body limit",
        [("r.Body = http.MaxBytesReader(w, r.Body, maxBody)", "_ = maxBody")],
        "TestErrors_BodyTooLarge413"),
    "unknown_fields_ok": (
        "Accepts unknown JSON fields",
        [("dec.DisallowUnknownFields()", "")],
        "TestValidation_UnknownField"),
    "alias_check_then_act": (
        "Checks the alias under one lock and inserts under another (race)",
        [("\t} else if _, ok := s.links[code]; ok {\n\t\treturn Link{}, errTaken\n\t}",
          "\t} else if _, ok := s.links[code]; ok {\n\t\treturn Link{}, errTaken\n\t} else {\n\t\ts.mu.Unlock()\n\t\ttime.Sleep(2 * time.Millisecond)\n\t\ts.mu.Lock()\n\t}")],
        "TestConcurrency_SameAliasOnceOnly"),
    "save_later": (
        "Saves in the background every 2 s instead of before answering",
        [("\tif err := s.saveLocked(); err != nil {\n\t\tdelete(s.links, code)",
          "\tif err := s.saveSoon(); err != nil {\n\t\tdelete(s.links, code)"),
         ("func (s *Store) listLocked() []Link {",
          "func (s *Store) saveSoon() error {\n\tgo func() {\n\t\ttime.Sleep(2 * time.Second)\n\t\ts.mu.Lock()\n\t\tdefer s.mu.Unlock()\n\t\t_ = s.saveLocked()\n\t}()\n\treturn nil\n}\n\nfunc (s *Store) listLocked() []Link {")],
        "TestPersistence_KillAndRecoverWithoutRestartGrace"),
    "plain_text_errors": (
        "Errors are plain text (http.Error) instead of JSON",
        [('func writeError(w http.ResponseWriter, status int, msg string) {\n\twriteJSON(w, status, map[string]string{"error": msg})',
          'func writeError(w http.ResponseWriter, status int, msg string) {\n\thttp.Error(w, msg, status)')],
        "TestErrors_JSONErrorFormat"),
    "runs_without_token": (
        "Starts without ADMIN_TOKEN (and so anyone with an empty token is admin)",
        [('\tif token == "" {\n\t\tfmt.Fprintln(os.Stderr, "ADMIN_TOKEN must be set")\n\t\tos.Exit(1)\n\t}', '\t_ = fmt.Sprint')],
        "TestAdmin_StartWithoutTokenFails"),
    "short_codes": (
        "Generates 6-character codes",
        [("codeLen    = 7", "codeLen    = 6")],
        "TestCore_CreateGeneratedCode"),
    "reserved_api_allowed": (
        "Lets someone take the alias api",
        [('if req.Alias != "" && (!aliasRE.MatchString(req.Alias) || req.Alias == reservedID) {',
          'if req.Alias != "" && !aliasRE.MatchString(req.Alias) {')],
        "TestValidation_BadAliases/reserved_api"),
}


def run_suite(binary):
    env = dict(os.environ, SHORTENER_BIN=binary)
    out = subprocess.run(["go", "test", "-count=1", "-json", "./..."], cwd=ROOT / "acceptance",
                         env=env, capture_output=True, text=True, timeout=900).stdout
    res = {}
    for line in out.splitlines():
        try:
            e = json.loads(line)
        except ValueError:
            continue
        if e.get("Test") and e["Action"] in ("pass", "fail"):
            res[e["Test"]] = e["Action"]
    parents = {t.split("/")[0] for t in res if "/" in t}
    return {t: a for t, a in res.items() if t not in parents}


def main():
    only = sys.argv[1:]
    holes = 0
    for name, (what, edits, target) in MUTANTS.items():
        if only and name not in only:
            continue
        src = REF
        for old, new in edits:
            if old not in src:
                sys.exit(f"{name}: patch text not found in reference: {old[:60]!r}")
            src = src.replace(old, new, 1)
        with tempfile.TemporaryDirectory() as d:
            shutil.copy(ROOT / "reference" / "go.mod", d)
            pathlib.Path(d, "main.go").write_text(src)
            b = subprocess.run(["go", "build", "-o", "shortener", "."], cwd=d, capture_output=True, text=True)
            if b.returncode:
                sys.exit(f"{name}: does not build:\n{b.stderr}")
            res = run_suite(os.path.join(d, "shortener"))
        failed = sorted(t for t, a in res.items() if a == "fail")
        caught = res.get(target) == "fail"
        holes += not caught
        print(f"{'CAUGHT ' if caught else 'MISSED!'} {name:22} {what}")
        print(f"        target {target}: {res.get(target, 'not run')}; {len(failed)}/{len(res)} tests fail")
        for t in failed:
            print(f"          - {t}")
    print(f"\n{holes} mutant(s) not caught by their target test")
    sys.exit(1 if holes else 0)


if __name__ == "__main__":
    main()
