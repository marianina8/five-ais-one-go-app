# Scorecard

**Provisional:**

- claude-opus-5-5/run1: 9 finding(s) still need a verdict
- claude-opus-5-5/run1: no blind review yet
- claude-sonnet-5-5/run1: 13 finding(s) still need a verdict
- claude-sonnet-5-5/run1: no blind review yet
- deepseek-v3-2/run1: no blind review yet
- gemini-3-1-pro/run1: the model's API failed (gemini: HTTP 429: {
  "error": {
    "code": 429,
    "message": "You exceeded your current quota, please check your pla …[trimmed]); re-run it before trusting this score
- gemini-3-1-pro/run1: no blind review yet
- gpt-6-1-sol/run1: 5 finding(s) still need a verdict
- gpt-6-1-sol/run1: no blind review yet

| # | Model | Works /40 | Bugs & security /20 | Readable /10 | Own tests /10 | Would I merge /20 | **Total /100** | Cost per run | Time per run | Model calls |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | gpt-6-1-sol | 40.0 | 20.0 | 10 | 10.0 | 0.0 | **80.0** | $0.05 | 1m18s | 3 |
| 2 | claude-opus-5-5 | 39.7 | 20.0 | 8 | 9.6 | 0.0 | **77.3** | $0.22 | 1m15s | 2 |
| 3 | claude-sonnet-5-5 | 40.0 | 20.0 | 6 | 8.8 | 0.0 | **74.8** | $0.14 | 1m00s | 3 |
| 4 | gemini-3-1-pro | 0.0 | 0.0 | 0 | 0.0 | 0.0 | **0.0** | $0.00 | 1m03s | 1 |
| 5 | deepseek-v3-2 | 0.0 | 0.0 | 0 | 0.0 | 0.0 | **0.0** | $0.01 | 1m32s | 3 |

Each model's points are the average of its runs. Readable Go is a rank across models. Cost and time are shown, not scored.

## Runs

| Run | Builds | Hidden tests | Works | Confirmed findings | Bugs | Style score | Own tests pass | Coverage | Merge | Cost | Time | Model calls | Said DONE |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| gpt-6-1-sol/run1 | yes | 55/55 | 40.0 | 0 | 20.0 | 7 | yes | 80.2% | – | $0.05 | 1m18s | 3 | yes |
| claude-opus-5-5/run1 | yes | 54/55 | 39.7 | 0 | 20.0 | 10 | yes | 76.4% | – | $0.22 | 1m15s | 2 | yes |
| claude-sonnet-5-5/run1 | yes | 55/55 | 40.0 | 0 | 20.0 | 12 | yes | 70.0% | – | $0.14 | 1m00s | 3 | yes |
| gemini-3-1-pro/run1 | no | 0/55 | 0.0 | 0 | 0.0 | 0 | no | 0.0% | – | $0.00 | 1m03s | 1 | no |
| deepseek-v3-2/run1 | no | 0/55 | 0.0 | 0 | 0.0 | 0 | no | 0.0% | – | $0.01 | 1m32s | 3 | no |
