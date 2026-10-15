# Scorecard

**Provisional:**

- baseline/run1: 53 finding(s) still need a verdict
- baseline/run1: no blind review yet
- baseline/run2: no blind review yet
- baseline/run3: 55 finding(s) still need a verdict
- baseline/run3: no blind review yet
- bonus/run1: 24 finding(s) still need a verdict
- bonus/run1: no blind review yet
- bonus/run2: 31 finding(s) still need a verdict
- bonus/run2: no blind review yet
- bonus/run3: 2 finding(s) still need a verdict
- bonus/run3: no blind review yet
- golang-pro/run1: no blind review yet
- golang-pro/run2: 2 finding(s) still need a verdict
- golang-pro/run2: no blind review yet
- golang-pro/run3: 1 finding(s) still need a verdict
- golang-pro/run3: no blind review yet
- karpathy-guidelines/run1: no blind review yet
- karpathy-guidelines/run2: 13 finding(s) still need a verdict
- karpathy-guidelines/run2: no blind review yet
- karpathy-guidelines/run3: 42 finding(s) still need a verdict
- karpathy-guidelines/run3: no blind review yet
- sharp-edges/run1: 1 finding(s) still need a verdict
- sharp-edges/run1: no blind review yet
- sharp-edges/run2: 49 finding(s) still need a verdict
- sharp-edges/run2: no blind review yet
- sharp-edges/run3: 33 finding(s) still need a verdict
- sharp-edges/run3: no blind review yet
- stack/run1: no blind review yet
- stack/run2: no blind review yet
- stack/run3: no blind review yet
- superpowers/run1: 18 finding(s) still need a verdict
- superpowers/run1: no blind review yet
- superpowers/run2: no blind review yet
- superpowers/run3: 8 finding(s) still need a verdict
- superpowers/run3: no blind review yet
- tdd/run1: 26 finding(s) still need a verdict
- tdd/run1: no blind review yet
- tdd/run2: 3 finding(s) still need a verdict
- tdd/run2: no blind review yet
- tdd/run3: 33 finding(s) still need a verdict
- tdd/run3: no blind review yet

| # | Model | Works /40 | Bugs & security /20 | Readable /10 | Own tests /10 | Would I merge /20 | **Total /100** | Cost per run | Time per run | Model calls |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | tdd | 36.1 | 20.0 | 8 | 6.2 | 0.0 | **70.3** | $0.40 | 8m57s | 32 |
| 2 | sharp-edges | 33.9 | 20.0 | 2 | 5.9 | 0.0 | **61.8** | $0.55 | 12m13s | 40 |
| 3 | bonus | 34.8 | 20.0 | 6 | 0.0 | 0.0 | **60.8** | $0.59 | 13m14s | 40 |
| 4 | golang-pro | 24.3 | 13.3 | 10 | 0.0 | 0.0 | **47.6** | $0.52 | 10m41s | 40 |
| 5 | karpathy-guidelines | 23.3 | 13.3 | 2 | 3.1 | 0.0 | **41.7** | $0.56 | 11m23s | 40 |
| 6 | baseline | 14.3 | 13.3 | 6 | 2.8 | 0.0 | **36.4** | $0.56 | 11m18s | 40 |
| 7 | superpowers | 12.2 | 13.3 | 2 | 4.5 | 0.0 | **32.1** | $0.45 | 6m10s | 30 |
| 8 | stack | 0.0 | 0.0 | 0 | 0.0 | 0.0 | **0.0** | $0.62 | 7m54s | 40 |

Each model's points are the average of its runs. Readable Go is a rank across models. Cost and time are shown, not scored.

## Runs

| Run | Builds | Hidden tests | Works | Confirmed findings | Bugs | Style score | Own tests pass | Coverage | Merge | Cost | Time | Model calls | Said DONE |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| tdd/run1 | yes | 52/55 | 38.3 | 0 | 20.0 | 7 | yes | 74.3% | – | $0.67 | 13m09s | 40 | yes |
| tdd/run2 | yes | 50/55 | 36.3 | 0 | 20.0 | 1 | no | 0.0% | – | $0.13 | 4m34s | 18 | yes |
| tdd/run3 | yes | 49/55 | 33.8 | 0 | 20.0 | 8 | yes | 74.7% | – | $0.39 | 9m08s | 37 | yes |
| sharp-edges/run1 | yes | 50/55 | 37.0 | 0 | 20.0 | 4 | no | 0.0% | – | $0.65 | 14m30s | 40 | yes |
| sharp-edges/run2 | yes | 44/55 | 28.3 | 0 | 20.0 | 14 | yes | 70.3% | – | $0.50 | 10m56s | 40 | yes |
| sharp-edges/run3 | yes | 50/55 | 36.3 | 0 | 20.0 | 15 | yes | 71.9% | – | $0.50 | 11m13s | 40 | yes |
| bonus/run1 | yes | 45/55 | 31.6 | 0 | 20.0 | 7 | no | 74.7% | – | $0.73 | 14m39s | 40 | no |
| bonus/run2 | yes | 51/55 | 37.3 | 0 | 20.0 | 17 | no | 78.5% | – | $0.51 | 14m07s | 40 | yes |
| bonus/run3 | yes | 48/55 | 35.4 | 0 | 20.0 | 3 | no | 0.0% | – | $0.52 | 10m59s | 40 | no |
| golang-pro/run1 | no | 0/55 | 0.0 | 0 | 0.0 | 0 | no | 0.0% | – | $0.43 | 8m57s | 40 | yes |
| golang-pro/run2 | yes | 50/55 | 36.3 | 0 | 20.0 | 1 | no | 0.0% | – | $0.53 | 11m35s | 40 | no |
| golang-pro/run3 | yes | 51/55 | 36.6 | 0 | 20.0 | 7 | no | 0.0% | – | $0.59 | 11m31s | 40 | yes |
| karpathy-guidelines/run1 | no | 0/55 | 0.0 | 0 | 0.0 | 0 | no | 0.0% | – | $0.43 | 7m06s | 40 | no |
| karpathy-guidelines/run2 | yes | 49/55 | 36.4 | 0 | 20.0 | 7 | yes | 73.7% | – | $0.67 | 15m25s | 40 | no |
| karpathy-guidelines/run3 | yes | 47/55 | 33.5 | 0 | 20.0 | 15 | no | 71.1% | – | $0.58 | 11m39s | 40 | yes |
| baseline/run1 | yes | 8/55 | 4.9 | 0 | 20.0 | 9 | no | 21.3% | – | $0.40 | 10m13s | 40 | no |
| baseline/run2 | no | 0/55 | 0.0 | 0 | 0.0 | 0 | no | 0.0% | – | $0.65 | 10m55s | 40 | yes |
| baseline/run3 | yes | 51/55 | 38.0 | 0 | 20.0 | 9 | yes | 66.8% | – | $0.64 | 12m48s | 40 | no |
| superpowers/run1 | yes | 49/55 | 36.0 | 0 | 20.0 | 10 | yes | 64.6% | – | $0.63 | 10m15s | 40 | yes |
| superpowers/run2 | no | 0/55 | 0.0 | 0 | 0.0 | 0 | no | 0.0% | – | $0.08 | 1m59s | 10 | yes |
| superpowers/run3 | yes | 1/55 | 0.6 | 0 | 20.0 | 12 | yes | 44.4% | – | $0.64 | 6m18s | 40 | no |
| stack/run1 | no | 0/55 | 0.0 | 0 | 0.0 | 0 | no | 0.0% | – | $0.59 | 5m35s | 40 | no |
| stack/run2 | no | 0/55 | 0.0 | 0 | 0.0 | 0 | no | 0.0% | – | $0.65 | 8m25s | 40 | no |
| stack/run3 | no | 0/55 | 0.0 | 0 | 0.0 | 0 | no | 0.0% | – | $0.62 | 9m44s | 40 | yes |
