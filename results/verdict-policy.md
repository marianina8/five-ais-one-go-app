# How findings were judged

Every possible bug, whether from a tool (go vet, staticcheck, gosec, errcheck, the race detector) or the review agent, gets one verdict in its run's `findings.json`. Only `confirmed` findings cost points. These rules were written before any verdict was given, and apply the same way to every model.

## Confirmed: a real defect in this code

A finding is confirmed when it points at code that is wrong, not just code that could be better:

1. **The code breaks the spec** ([`spec/SPEC.md`](../spec/SPEC.md)): it accepts input the spec says to reject, answers with the wrong status, loses data the spec says must be saved, or misbehaves under concurrent requests.
2. **A data race or lost update**, shown by the race detector or by the code itself.
3. **Error handling that gives a wrong result or loses data:** for example, an ignored write, sync or close error on the data file before it's renamed into place, or a load error that makes the service start empty and then overwrite the saved file.
4. **A security problem within the spec's scope:** an admin check that can be bypassed, a token compared in a way that leaks it, no request body limit, or an HTTP server with no timeouts (a slow client can hold connections open forever).
5. **A test that can't fail:** one that checks a value it built itself, or asserts nothing.

## Rejected: not a defect

1. **The spec requires it.** Creating links without a token, counting a visit on every follow, and saving before every response are all in the spec, so they aren't bugs.
2. **A feature the spec didn't ask for:** rate limiting, logging, metrics, graceful shutdown, rejecting `user:pass@` URLs, re-validating a hand-edited data file, and so on.
3. **Style, clarity and missing tests.** Those are scored elsewhere: style and complexity in Readable Go, coverage in Its own tests, clarity in the blind "Would I merge it?" review. Counting them here too would score them twice.
4. **An ignored error with no practical effect:** encoding a response to a client that has gone away, closing a file that was only read, test code, cleanup.
5. **A tool warning that doesn't apply:** gosec G304 (the data file path comes from the operator's `-data` flag), G306 (data file permissions; the spec says nothing about who may read the file), G404 (codes from `math/rand`: links are public once shared, and the spec only asks for random codes).

## Duplicate

The same problem reported twice in one run (by two tools, or by a tool and the agent) is confirmed once; the other report is `duplicate`.

## Severity

A confirmed tool finding costs 2 points. A confirmed agent finding costs what its severity says (high 4, medium 2, low 1). The agent's severity stands unless it's clearly wrong; any change is written in the finding's `note`.

## Who decides

Claude drafted a verdict and a one-line reason (`note`) for every finding by reading the code. Marian confirmed or changed each one before the scorecard was finalised.
