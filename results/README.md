# Results

Each run has its own folder:

```
results/<model>/<run>/
  workspace/       the code exactly as the model left it
  transcript.md    every model call and tool call, in order (no API keys)
  stats.json       model calls, tokens, cost, time, whether it finished
  measure.json     what the scorer measured: hidden tests, linters, race detector, coverage
  findings.json    every possible bug from the tools and the review agent, with a human verdict
```

`stats.json` also says why the run stopped: `done` (the model said DONE), `steps` (out of model calls), `time` (out of time) or `error` (the vendor's API kept failing; such a run is re-run, not scored).

Plus `scorecard.md` (the final scores) and `blind/` (the letter-to-model key `map.json` and the blind review scores `reviews.json`).

The review agent's own reports (`review.md`, `review-log.txt`, `review-stats.json`) are kept for reference. They weren't scored: see [verdict-policy.md](verdict-policy.md).
