# Results

Empty until the contest runs. Each run will get its own folder:

```
results/<model>/<run>/
  workspace/       the code exactly as the model left it
  transcript.md    every model call and tool call, in order (no API keys)
  stats.json       model calls, tokens, cost, time, whether it finished
  measure.json     what the scorer measured: hidden tests, linters, race detector, coverage
  findings.json    every possible bug from the tools and the review agent, with a human verdict
  review.md        the review agent's full review
```

Plus `scorecard.md` (the final scores) and `blind/` (the letter-to-model mapping, published after the blind review).
