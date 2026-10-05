# Scorecard

**Provisional:**

- claude-opus-5-5/run1: 11 finding(s) still need a verdict
- claude-opus-5-5/run1: no blind review yet
- claude-opus-5-5/run2: 14 finding(s) still need a verdict
- claude-opus-5-5/run2: no blind review yet
- claude-sonnet-5-5/run1: 9 finding(s) still need a verdict
- claude-sonnet-5-5/run1: no blind review yet
- claude-sonnet-5-5/run2: 10 finding(s) still need a verdict
- claude-sonnet-5-5/run2: no blind review yet
- deepseek-v3-2/run1: 9 finding(s) still need a verdict
- deepseek-v3-2/run1: no blind review yet
- deepseek-v3-2/run2: 17 finding(s) still need a verdict
- deepseek-v3-2/run2: no blind review yet
- gemini-3-1-pro/run1: the model's API failed (gemini: HTTP 429: {
  "error": {
    "code": 429,
    "message": "You exceeded your current quota, please check your pla …[trimmed]); re-run it before trusting this score
- gemini-3-1-pro/run1: no blind review yet
- gemini-3-1-pro/run2: the model's API failed (gemini: HTTP 429: {
  "error": {
    "code": 429,
    "message": "You exceeded your current quota, please check your pla …[trimmed]); re-run it before trusting this score
- gemini-3-1-pro/run2: no blind review yet
- gpt-6-1-sol/run1: 13 finding(s) still need a verdict
- gpt-6-1-sol/run1: no blind review yet
- gpt-6-1-sol/run2: 5 finding(s) still need a verdict
- gpt-6-1-sol/run2: no blind review yet

| # | Model | Works /40 | Bugs & security /20 | Readable /10 | Own tests /10 | Would I merge /20 | **Total /100** | Cost per run | Time per run | Model calls |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | gpt-6-1-sol | 40.0 | 20.0 | 10 | 10.0 | 0.0 | **80.0** | $0.33 | 5m47s | 10 |
| 2 | claude-sonnet-5-5 | 40.0 | 20.0 | 8 | 9.4 | 0.0 | **77.4** | $0.16 | 1m05s | 4 |
| 3 | claude-opus-5-5 | 40.0 | 20.0 | 4 | 9.0 | 0.0 | **73.0** | $0.28 | 1m19s | 4 |
| 4 | deepseek-v3-2 | 34.4 | 20.0 | 8 | 9.8 | 0.0 | **72.2** | $0.59 | 16m03s | 39 |
| 5 | gemini-3-1-pro | 0.0 | 0.0 | 0 | 0.0 | 0.0 | **0.0** | $0.00 | 1m03s | 1 |

Each model's points are the average of its runs. Readable Go is a rank across models. Cost and time are shown, not scored.

## Runs

| Run | Builds | Hidden tests | Works | Confirmed findings | Bugs | Style score | Own tests pass | Coverage | Merge | Cost | Time | Model calls | Said DONE |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| gpt-6-1-sol/run1 | yes | 55/55 | 40.0 | 0 | 20.0 | 6 | yes | 92.7% | – | $0.42 | 7m03s | 10 | yes |
| gpt-6-1-sol/run2 | yes | 55/55 | 40.0 | 0 | 20.0 | 7 | yes | 83.6% | – | $0.25 | 4m32s | 9 | yes |
| claude-sonnet-5-5/run1 | yes | 55/55 | 40.0 | 0 | 20.0 | 9 | yes | 74.4% | – | $0.20 | 1m08s | 6 | yes |
| claude-sonnet-5-5/run2 | yes | 55/55 | 40.0 | 0 | 20.0 | 9 | yes | 75.9% | – | $0.13 | 1m03s | 3 | yes |
| claude-opus-5-5/run1 | yes | 55/55 | 40.0 | 0 | 20.0 | 9 | yes | 74.5% | – | $0.25 | 1m15s | 3 | yes |
| claude-opus-5-5/run2 | yes | 55/55 | 40.0 | 0 | 20.0 | 14 | yes | 69.5% | – | $0.31 | 1m24s | 4 | yes |
| deepseek-v3-2/run1 | yes | 47/55 | 33.1 | 0 | 20.0 | 6 | yes | 77.9% | – | $0.54 | 14m50s | 38 | yes |
| deepseek-v3-2/run2 | yes | 50/55 | 35.6 | 0 | 20.0 | 12 | yes | 79.5% | – | $0.65 | 17m16s | 40 | yes |
| gemini-3-1-pro/run1 | no | 0/55 | 0.0 | 0 | 0.0 | 0 | no | 0.0% | – | $0.00 | 1m03s | 1 | no |
| gemini-3-1-pro/run2 | no | 0/55 | 0.0 | 0 | 0.0 | 0 | no | 0.0% | – | $0.00 | 1m03s | 1 | no |
