# P1 · T1 — reach of later-step passages with a notebook (MuSiQue)

Memory: 4757 paragraphs. Targets: gold passages of steps that depend on earlier steps (middle: not last; final: states the answer). Notebook = silver facts of the earlier steps (the sentence of their gold passage that contains their answer); frontier = the answers of the steps this one depends on.

Silver facts: 382 exact, 4 partial, 0 none. Targets with an incomplete notebook: 0.

Reach@K = share of targets whose gold passage is in the top K (95% CI resampling questions).

## middle steps (n = 63)

| search | reach@30 | reach@100 | reach@300 |
|---|---|---|---|
| vector · question alone (today) | 0.62 [0.50–0.74] | 0.78 [0.66–0.88] | 0.90 [0.83–0.97] |
| keywords · question alone | 0.33 [0.22–0.46] | 0.48 [0.35–0.60] | 0.65 [0.54–0.76] |
| hybrid · question alone | 0.51 [0.39–0.63] | 0.75 [0.62–0.86] | 0.87 [0.79–0.95] |
| vector · question + notebook facts | 0.90 [0.84–0.97] | 0.98 [0.95–1.00] | 0.98 [0.95–1.00] |
| vector · notebook facts alone | 0.73 [0.62–0.83] | 0.90 [0.82–0.97] | 0.97 [0.92–1.00] |
| keywords · question + frontier entity | 0.84 [0.76–0.92] | 0.92 [0.85–0.98] | 0.97 [0.92–1.00] |
| keywords · frontier entity alone | 0.94 [0.88–0.98] | 0.98 [0.95–1.00] | 0.98 [0.95–1.00] |
| hybrid · question + facts + frontier | 0.89 [0.82–0.96] | 0.97 [0.92–1.00] | 1.00 [1.00–1.00] |
| title lookup · frontier entity | 0.17 [0.08–0.28] | 0.17 [0.08–0.28] | 0.17 [0.08–0.28] |
| (upper bound) vector · resolved sub-question | 0.94 [0.88–0.98] | 0.98 [0.95–1.00] | 1.00 [1.00–1.00] |

## final steps (n = 150)

| search | reach@30 | reach@100 | reach@300 |
|---|---|---|---|
| vector · question alone (today) | 0.88 [0.83–0.93] | 0.97 [0.95–0.99] | 0.99 [0.97–1.00] |
| keywords · question alone | 0.51 [0.42–0.59] | 0.58 [0.50–0.66] | 0.71 [0.63–0.78] |
| hybrid · question alone | 0.73 [0.66–0.81] | 0.92 [0.87–0.96] | 0.97 [0.95–0.99] |
| vector · question + notebook facts | 0.98 [0.96–1.00] | 1.00 [1.00–1.00] | 1.00 [1.00–1.00] |
| vector · notebook facts alone | 0.73 [0.65–0.79] | 0.89 [0.83–0.93] | 0.95 [0.91–0.98] |
| keywords · question + frontier entity | 0.95 [0.91–0.99] | 0.98 [0.95–1.00] | 0.99 [0.98–1.00] |
| keywords · frontier entity alone | 0.94 [0.90–0.97] | 0.97 [0.95–0.99] | 0.97 [0.95–0.99] |
| hybrid · question + facts + frontier | 0.99 [0.98–1.00] | 1.00 [1.00–1.00] | 1.00 [1.00–1.00] |
| title lookup · frontier entity | 0.27 [0.21–0.35] | 0.27 [0.20–0.35] | 0.27 [0.20–0.35] |
| (upper bound) vector · resolved sub-question | 0.99 [0.98–1.00] | 1.00 [1.00–1.00] | 1.00 [1.00–1.00] |

