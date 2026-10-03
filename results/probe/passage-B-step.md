# Probe report: passage-B-step

Step-framed bridge noul (with generic step examples) plus answers_query from set A.

673 pairs · 673 paid calls · 0 cache hits · 736808 input tokens (~US$0.031) · 20s

AUC (0.5 = no separation, 1 = perfect). Reference row: embedding rank alone.

| feature | all: gold vs hard-neg | koblex: gold vs hard-neg | musique: gold vs hard-neg | musique: FINAL vs hard-neg | musique: BRIDGE vs hard-neg | musique: gold vs unanswerable-top |
|---|---|---|---|---|---|---|
| _embedding rank_ | 0.561 | 0.552 | 0.588 | 0.511 | 0.632 | 0.391 |
| `answers_query` | 0.627 | 0.763 | 0.533 | 0.761 | 0.402 | 0.573 |
| `supports_a_step` | 0.762 | 0.831 | 0.716 | 0.666 | 0.745 | 0.688 |

## Combined score (logistic regression, trained on one dataset, scored on the other)

| combination | train koblex → musique gold vs hard-neg | train musique → koblex gold vs hard-neg | musique BRIDGE vs hard-neg (train koblex) |
|---|---|---|---|
| JEV answers | 0.712 | 0.812 | 0.734 |
| JEV answers + embedding rank | 0.781 | 0.850 | 0.798 |

## Mean answer by passage role

| feature | final | bridge | gold (statutes) | hard-neg | unanswerable-top |
|---|---|---|---|---|---|
| `answers_query` | 0.32 | 0.08 | 0.40 | 0.17 | 0.11 |
| `supports_a_step` | 0.60 | 0.67 | 0.86 | 0.52 | 0.46 |
