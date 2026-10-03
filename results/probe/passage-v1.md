# Probe report: passage-v1

Baseline: the questions the harness used in v1 (one 4-tier relevance choice + a sufficiency noul), on the agnostic state. Kept to compare every new set against.

673 pairs · 673 paid calls · 0 cache hits · 492509 input tokens (~US$0.021) · 20s

AUC (0.5 = no separation, 1 = perfect). Reference row: embedding rank alone.

| feature | all: gold vs hard-neg | koblex: gold vs hard-neg | musique: gold vs hard-neg | musique: FINAL vs hard-neg | musique: BRIDGE vs hard-neg | musique: gold vs unanswerable-top |
|---|---|---|---|---|---|---|
| _embedding rank_ | 0.561 | 0.552 | 0.588 | 0.511 | 0.632 | 0.391 |
| `relevance=direct` | 0.681 | 0.802 | 0.539 | 0.782 | 0.398 | 0.624 |
| `relevance=high` | 0.651 | 0.629 | 0.700 | 0.636 | 0.737 | 0.723 |
| `relevance=irrelevant` | 0.276 | 0.151 | 0.255 | 0.252 | 0.256 | 0.316 |
| `relevance=weak` | 0.279 | 0.187 | 0.493 | 0.438 | 0.524 | 0.444 |
| `sufficient` | 0.640 | 0.728 | 0.565 | 0.787 | 0.436 | 0.628 |

## Combined score (logistic regression, trained on one dataset, scored on the other)

| combination | train koblex → musique gold vs hard-neg | train musique → koblex gold vs hard-neg | musique BRIDGE vs hard-neg (train koblex) |
|---|---|---|---|
| JEV answers | 0.724 | 0.774 | 0.708 |
| JEV answers + embedding rank | 0.794 | 0.800 | 0.783 |

## Mean answer by passage role

| feature | final | bridge | gold (statutes) | hard-neg | unanswerable-top |
|---|---|---|---|---|---|
| `relevance=direct` | 0.29 | 0.07 | 0.49 | 0.15 | 0.09 |
| `relevance=high` | 0.26 | 0.42 | 0.37 | 0.23 | 0.20 |
| `relevance=irrelevant` | 0.22 | 0.23 | 0.01 | 0.26 | 0.41 |
| `relevance=weak` | 0.23 | 0.28 | 0.13 | 0.36 | 0.30 |
| `sufficient` | 0.31 | 0.10 | 0.32 | 0.14 | 0.11 |
