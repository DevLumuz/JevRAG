# Probe report: passage-C-graded

Graded contribution (score) and passage role (choice, use probabilities, never argmax).

673 pairs · 673 paid calls · 0 cache hits · 625090 input tokens (~US$0.026) · 20s

AUC (0.5 = no separation, 1 = perfect). Reference row: embedding rank alone.

| feature | all: gold vs hard-neg | koblex: gold vs hard-neg | musique: gold vs hard-neg | musique: FINAL vs hard-neg | musique: BRIDGE vs hard-neg | musique: gold vs unanswerable-top |
|---|---|---|---|---|---|---|
| _embedding rank_ | 0.561 | 0.552 | 0.588 | 0.511 | 0.632 | 0.391 |
| `contribution` | 0.735 | 0.816 | 0.686 | 0.739 | 0.655 | 0.671 |
| `role=background` | 0.336 | 0.297 | 0.442 | 0.433 | 0.447 | 0.447 |
| `role=final_answer` | 0.647 | 0.814 | 0.516 | 0.790 | 0.358 | 0.582 |
| `role=linking_fact` | 0.559 | 0.437 | 0.687 | 0.483 | 0.805 | 0.664 |
| `role=off_target` | 0.221 | 0.142 | 0.234 | 0.305 | 0.193 | 0.322 |

## Combined score (logistic regression, trained on one dataset, scored on the other)

| combination | train koblex → musique gold vs hard-neg | train musique → koblex gold vs hard-neg | musique BRIDGE vs hard-neg (train koblex) |
|---|---|---|---|
| JEV answers | 0.746 | 0.834 | 0.760 |
| JEV answers + embedding rank | 0.808 | 0.879 | 0.824 |

## Mean answer by passage role

| feature | final | bridge | gold (statutes) | hard-neg | unanswerable-top |
|---|---|---|---|---|---|
| `contribution` | 2.57 | 2.24 | 3.24 | 2.06 | 1.79 |
| `role=background` | 0.08 | 0.09 | 0.12 | 0.19 | 0.12 |
| `role=final_answer` | 0.33 | 0.05 | 0.76 | 0.28 | 0.10 |
| `role=linking_fact` | 0.35 | 0.70 | 0.10 | 0.20 | 0.40 |
| `role=off_target` | 0.23 | 0.16 | 0.02 | 0.32 | 0.38 |
