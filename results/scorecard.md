# Scorecard

| # | Model | Works /40 | Bugs & security /20 | Readable /10 | Own tests /10 | Would I merge /20 | **Total /100** | Cost per run | Time per run | Model calls |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | gpt-6-1-sol | 40.0 | 11.0 | 10 | 10.0 | 10.0 | **81.0** | $0.33 | 5m47s | 10 |
| 2 | claude-sonnet-5-5 | 40.0 | 4.0 | 6 | 9.4 | 10.0 | **69.4** | $0.16 | 1m05s | 4 |
| 3 | deepseek-v3-2 | 33.5 | 6.0 | 6 | 5.0 | 17.0 | **67.5** | $0.59 | 16m03s | 39 |
| 4 | gemini-3-1-pro | 39.3 | 1.0 | 8 | 8.9 | 8.0 | **65.2** | $1.48 | 4m56s | 36 |
| 5 | claude-opus-5-5 | 40.0 | 3.0 | 2 | 8.9 | 8.0 | **61.9** | $0.28 | 1m19s | 4 |

Each model's points are the average of its runs. Readable Go is a rank across models. Cost and time are shown, not scored.

## Runs

| Run | Builds | Hidden tests | Works | Confirmed findings | Bugs | Style score | Own tests pass | Coverage | Merge | Cost | Time | Model calls | Said DONE |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| gpt-6-1-sol/run1 | yes | 55/55 | 40.0 | 5 | 10.0 | 6 | yes | 91.6% | 14 | $0.42 | 7m03s | 10 | yes |
| gpt-6-1-sol/run2 | yes | 55/55 | 40.0 | 4 | 12.0 | 7 | yes | 82.1% | 6 | $0.25 | 4m32s | 9 | yes |
| claude-sonnet-5-5/run1 | yes | 55/55 | 40.0 | 8 | 4.0 | 9 | yes | 74.4% | 10 | $0.20 | 1m08s | 6 | yes |
| claude-sonnet-5-5/run2 | yes | 55/55 | 40.0 | 8 | 4.0 | 9 | yes | 75.9% | 10 | $0.13 | 1m03s | 3 | yes |
| deepseek-v3-2/run1 | yes | 46/55 | 32.0 | 8 | 4.0 | 6 | no | 78.8% | 20 | $0.54 | 14m50s | 38 | yes |
| deepseek-v3-2/run2 | yes | 49/55 | 35.0 | 6 | 8.0 | 12 | yes | 81.2% | 14 | $0.65 | 17m16s | 40 | yes |
| gemini-3-1-pro/run1 | yes | 53/55 | 39.3 | 10 | 0.0 | 5 | yes | 80.7% | 6 | $1.66 | 5m46s | 40 | yes |
| gemini-3-1-pro/run2 | yes | 53/55 | 39.3 | 9 | 2.0 | 10 | yes | 62.5% | 10 | $1.30 | 4m06s | 32 | yes |
| claude-opus-5-5/run1 | yes | 55/55 | 40.0 | 9 | 2.0 | 9 | yes | 75.0% | 10 | $0.25 | 1m15s | 3 | yes |
| claude-opus-5-5/run2 | yes | 55/55 | 40.0 | 8 | 4.0 | 14 | yes | 67.2% | 6 | $0.31 | 1m24s | 4 | yes |
