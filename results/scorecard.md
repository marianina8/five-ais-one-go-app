# Scorecard

**Provisional:**

- claude-opus-5-5/run1: no blind review yet
- claude-opus-5-5/run2: no blind review yet
- claude-sonnet-5-5/run1: no blind review yet
- claude-sonnet-5-5/run2: no blind review yet
- deepseek-v3-2/run1: no blind review yet
- deepseek-v3-2/run2: no blind review yet
- gemini-3-1-pro/run1: no blind review yet
- gemini-3-1-pro/run2: no blind review yet
- gpt-6-1-sol/run1: 13 finding(s) still need a verdict
- gpt-6-1-sol/run1: no blind review yet
- gpt-6-1-sol/run2: 5 finding(s) still need a verdict
- gpt-6-1-sol/run2: no blind review yet

| # | Model | Works /40 | Bugs & security /20 | Readable /10 | Own tests /10 | Would I merge /20 | **Total /100** | Cost per run | Time per run | Model calls |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | gpt-6-1-sol | 40.0 | 20.0 | 10 | 10.0 | 0.0 | **80.0** | $0.33 | 5m47s | 10 |
| 2 | claude-sonnet-5-5 | 40.0 | 18.0 | 6 | 9.4 | 0.0 | **73.4** | $0.16 | 1m05s | 4 |
| 3 | claude-opus-5-5 | 40.0 | 19.0 | 2 | 9.0 | 0.0 | **70.0** | $0.28 | 1m19s | 4 |
| 4 | gemini-3-1-pro | 39.3 | 13.5 | 8 | 8.7 | 0.0 | **69.5** | $1.48 | 4m56s | 36 |
| 5 | deepseek-v3-2 | 34.4 | 12.5 | 6 | 9.8 | 0.0 | **62.7** | $0.59 | 16m03s | 39 |

Each model's points are the average of its runs. Readable Go is a rank across models. Cost and time are shown, not scored.

## Runs

| Run | Builds | Hidden tests | Works | Confirmed findings | Bugs | Style score | Own tests pass | Coverage | Merge | Cost | Time | Model calls | Said DONE |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| gpt-6-1-sol/run1 | yes | 55/55 | 40.0 | 0 | 20.0 | 6 | yes | 92.7% | – | $0.42 | 7m03s | 10 | yes |
| gpt-6-1-sol/run2 | yes | 55/55 | 40.0 | 0 | 20.0 | 7 | yes | 83.6% | – | $0.25 | 4m32s | 9 | yes |
| claude-sonnet-5-5/run1 | yes | 55/55 | 40.0 | 1 | 18.0 | 9 | yes | 74.4% | – | $0.20 | 1m08s | 6 | yes |
| claude-sonnet-5-5/run2 | yes | 55/55 | 40.0 | 1 | 18.0 | 9 | yes | 75.9% | – | $0.13 | 1m03s | 3 | yes |
| claude-opus-5-5/run1 | yes | 55/55 | 40.0 | 1 | 18.0 | 9 | yes | 74.5% | – | $0.25 | 1m15s | 3 | yes |
| claude-opus-5-5/run2 | yes | 55/55 | 40.0 | 0 | 20.0 | 14 | yes | 69.5% | – | $0.31 | 1m24s | 4 | yes |
| gemini-3-1-pro/run1 | yes | 53/55 | 39.3 | 3 | 15.0 | 5 | yes | 76.8% | – | $1.66 | 5m46s | 40 | yes |
| gemini-3-1-pro/run2 | yes | 53/55 | 39.3 | 4 | 12.0 | 10 | yes | 61.9% | – | $1.30 | 4m06s | 32 | yes |
| deepseek-v3-2/run1 | yes | 47/55 | 33.1 | 5 | 12.0 | 6 | yes | 77.9% | – | $0.54 | 14m50s | 38 | yes |
| deepseek-v3-2/run2 | yes | 50/55 | 35.6 | 4 | 13.0 | 12 | yes | 79.5% | – | $0.65 | 17m16s | 40 | yes |
