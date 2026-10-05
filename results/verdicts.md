# Verdicts to confirm

Every finding from the tools and the review agent got a draft verdict under [the verdict policy](verdict-policy.md), written by reviewers who saw each project under a random code. **Only confirmed findings cost points.** This page lists every confirmed finding, the duplicates, and the judgement calls worth a second look. The full list, with a reason for every rejection, is in each run's `findings.json`.

## Summary

| Run | Findings | Confirmed | Duplicate | Rejected | Points lost (of 20) |
|---|---|---|---|---|---|
| GPT-6.1 Sol run1 | 34 | 0 | 0 | 34 | 0 |
| GPT-6.1 Sol run2 | 23 | 0 | 0 | 23 | 0 |
| Claude Sonnet 5.5 run1 | 32 | 1 | 1 | 30 | 2 |
| Claude Sonnet 5.5 run2 | 31 | 1 | 1 | 29 | 2 |
| Claude Opus 5.5 run1 | 30 | 1 | 1 | 28 | 2 |
| Claude Opus 5.5 run2 | 35 | 0 | 0 | 35 | 0 |
| Gemini 3.1 Pro run1 | 41 | 3 | 2 | 36 | 5 |
| Gemini 3.1 Pro run2 | 49 | 4 | 3 | 42 | 8 |
| DeepSeek V3.2 run1 | 42 | 5 | 6 | 31 | 8 |
| DeepSeek V3.2 run2 | 46 | 4 | 2 | 40 | 7 |

## Confirmed findings

### GPT-6.1 Sol run1

None.


### GPT-6.1 Sol run2

None.


### Claude Sonnet 5.5 run1

- **T07** (gosec G114, tool, 2 points): main.go:185 http.ListenAndServe uses a server with no timeouts.

### Claude Sonnet 5.5 run2

- **T08** (gosec G112, tool, 2 points): main.go:189 http.Server has no ReadHeaderTimeout or any other timeout, so slow clients can hold connections forever.

### Claude Opus 5.5 run1

- **T09** (gosec G114, tool, 2 points): main.go:194 http.ListenAndServe uses a server with no timeouts.

### Claude Opus 5.5 run2

None.


### Gemini 3.1 Pro run1

- **T16** (gosec G114, tool, 2 points): http.ListenAndServe with no server timeouts (main.go:30).
- **A07** (agent, medium): Admin token is compared with != rather than in constant time, which can leak it through timing (server.go:28-29).
- **A19** (agent, low): TestRFC3339 marshals a time it built itself and accepts any Z or +, so it cannot fail (time_test.go:10-16).

### Gemini 3.1 Pro run2

- **T01** (race DATA RACE, tool, 2 points): store.go:116 Visit increments l.Visits under the lock while List/Create return live *Link pointers that server.go:137/107 JSON-encode after unlock: a real data race.
- **T20** (gosec G114, tool, 2 points): main.go:39 http.ListenAndServe uses a server with no timeouts.
- **A01** (agent, medium): server.go:178-191 re-wraps handler JSON errors on 404/405; verified GET /nope returns {"error":"{\"error\":\"not found\"}"}, which breaks the spec's error body format.
- **A09** (agent, medium): server.go:28-31 compares the bearer token with ==, not constant-time, which the policy counts as a token-leaking comparison.

### DeepSeek V3.2 run1

- **T05** (gosec G114, tool, 2 points): http.ListenAndServe with no server timeouts (main.go:30).
- **A01** (agent, medium): Severity changed from high to medium. Delete removes the link from memory and never restores it when saveLocked fails, so a request answered 500 still deletes in memory and memory/disk diverge (store.go:230-243); needs a disk failure, so lowered from high to medium. Severity set to medium for both projects with this failed-save-on-delete bug: it only happens when a save fails.
- **A04** (agent, low): Severity changed from medium to low. Temp file is written with os.WriteFile and renamed with no fsync (store.go:81-88, 107-114), so an OS crash can leave an empty data.json that fails to parse at startup, breaking the crash-safety rule. Severity set to low for both projects with this missing fsync: only an OS crash or power loss, not a killed process, can leave the bad file.
- **A07** (agent, medium): Admin token is compared with == rather than in constant time, which can leak it through timing (main.go:99).
- **A25** (agent, low): TestMain_EnvironmentValidation only calls t.Log and asserts nothing, so it cannot fail (handler_test.go:326-338).

### DeepSeek V3.2 run2

- **T15** (gosec G114, tool, 2 points): main.go:28 http.ListenAndServe uses a server with no timeouts.
- **A01** (agent, medium): Severity changed from high to medium. store.go:175 Delete rollback reinserts &Link{Code: code}, losing URL/created_at/visits (verified: empty link after failed save; next successful save persists it). Severity set to medium for both projects with this failed-save-on-delete bug: it only happens when a save fails.
- **A06** (agent, low): store.go:84 os.WriteFile without fsync before os.Rename can leave an empty/truncated data file after a power loss, which then fails to load (spec: crash must never leave an unstartable file). Severity set to low for both projects with this missing fsync: only an OS crash or power loss, not a killed process, can leave the bad file.
- **A08** (agent, medium): handler.go:182 compares the admin token with ==, not constant-time; policy lists leaky token comparison as a defect.

## Duplicates

Real problems already counted elsewhere: by a failing hidden test ("Does it work?") or by another finding in the same run.

- Claude Sonnet 5.5 run1 **A11**: Same missing-server-timeouts issue as T07 (main.go:185).
- Claude Sonnet 5.5 run2 **A09**: main.go:189 same missing-timeouts problem as T08.
- Claude Opus 5.5 run1 **A10**: Same missing server timeouts as T09 (main.go:194); graceful shutdown is not required.
- Gemini 3.1 Pro run1 **A10**: Real, but already scored: the hidden test TestValidation_MalformedJSON/trailing_data fails for this. Only the first JSON value is decoded, so a malformed body like {...}garbage gets 201 instead of the spec-required 400 (server.go:45-55).
- Gemini 3.1 Pro run1 **A13**: Same missing-timeouts issue as T16 (main.go:30); graceful shutdown is out of scope.
- Gemini 3.1 Pro run2 **A06**: Real, but already scored: the hidden test TestValidation_MalformedJSON/trailing_data fails for this. server.go:54-63 decodes one value and ignores the rest; verified `{"url":"http://a.com"} garbage` returns 201 instead of the spec's 400 for malformed JSON.
- Gemini 3.1 Pro run2 **A14**: Same missing server timeouts as T20 (main.go:39); graceful shutdown is not required.
- Gemini 3.1 Pro run2 **A16**: Same data race on Link.Visits as race-detector finding T01 (store.go:116 vs encoding of live pointers).
- DeepSeek V3.2 run1 **A03**: Real, but already scored: the hidden test TestErrors_BodyTooLarge413 fails for this. MaxBytesReader errors fall into the generic decode branch and return 400 instead of the spec-required 413 for bodies over 1 MiB (main.go:112-115).
- DeepSeek V3.2 run1 **A12**: Real, but already scored: the hidden test TestValidation_UnknownField fails for this. Decoder has no DisallowUnknownFields, so unknown fields get 201 instead of the spec-required 400 (main.go:107-115).
- DeepSeek V3.2 run1 **A15**: Same missing-timeouts issue as T05 (main.go:30).
- DeepSeek V3.2 run1 **A16**: Same root cause as A18, the async per-redirect goroutine (main.go:188); the rate-limit and performance parts are out of scope.
- DeepSeek V3.2 run1 **A18**: Real, but already scored: the hidden test TestPersistence_KillAndRecoverWithoutRestartGrace and TestConcurrency_ParallelVisitsExact fails for this. Visit increment and save run in a fire-and-forget goroutine after the 302 is sent, so the visit is not saved before the response (spec) and can be lost (main.go:188-194).
- DeepSeek V3.2 run1 **A31**: Real, but already scored: the hidden test TestErrors_MethodNotAllowed fails for this. Unsupported methods on known paths (e.g. PUT /api/links, POST /{code}) fall to the default branch and return 404 instead of the spec-required 405 (main.go:65-77).
- DeepSeek V3.2 run2 **A11**: Real, but already scored: the hidden test TestValidation_UnknownField fails for this. handler.go:63 json.Unmarshal accepts unknown fields (verified: {"alais":..} returns 201) but the spec requires 400; severity raised to medium because it is a spec status-code violation, not an optional strictness.
- DeepSeek V3.2 run2 **A15**: Same missing-server-timeouts issue as T15 (main.go:28).

## Judgement calls worth a second look

These are the verdicts where a reasonable reviewer could go the other way. Change any of them in the run's `findings.json` (set `verdict`, and say why in `note`), then run `score tally`.

1. **The admin token is compared with `==` instead of in constant time** (confirmed, medium): Gemini run1 A07 and run2 A09, DeepSeek run1 A07 and run2 A08. In principle the response time leaks how much of a guessed token was right. Over a network that's hard to exploit, so rating it low (−1 instead of −2) would also be defensible. Whichever you choose, apply it to all four.
2. **No server timeouts** (confirmed, 2 points as a tool finding): Sonnet runs 1 and 2, Opus run1, Gemini runs 1 and 2, DeepSeek runs 1 and 2. GPT and Opus run2 set timeouts. Opus run2 set only `ReadHeaderTimeout`, which blocks slow-header attacks, so its agent finding asking for more (A11) was rejected.
3. **The data file isn't fsynced before the rename** (confirmed, low): DeepSeek run1 A04 and run2 A06. Killing the process is safe (the hidden kill tests pass); only an OS crash or power loss could leave an empty file.
4. **Gemini run2 error format** (A01, confirmed, medium): the 404/405 wrapper double-wraps errors, so `GET /nope` returns `{"error":"{\"error\":\"not found\"}"}`. The hidden test only checks that `error` is a string, so it passed. The spec's error format is clearly broken, which is why it's confirmed here and not counted as a duplicate.
5. **GPT returns 500 when only the final directory fsync fails** (rejected): runs 1 and 2 (A02 and A01). The change is saved and memory matches the file, but the client is told it failed, so a retry with the same alias gets 409. It was rejected because nothing is lost or inconsistent. If you count "told it failed when it succeeded" as wrong error handling, confirm both.
6. **DeepSeek run2 checks the token before the method** (A13, rejected): `PUT /api/links/x` without a token gets 401, not 405. The spec doesn't say which check comes first.
