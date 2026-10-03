# Probe report: passage-D-combo

Best-of round 1 in one request: step-framed bridge noul (B) + graded contribution score and role choice (C).

673 pairs · 673 paid calls · 0 cache hits · 779207 input tokens (~US$0.033) · 21s

AUC (0.5 = no separation, 1 = perfect). Reference row: embedding rank alone.

| feature | all: gold vs hard-neg | koblex: gold vs hard-neg | musique: gold vs hard-neg | musique: FINAL vs hard-neg | musique: BRIDGE vs hard-neg | musique: gold vs unanswerable-top |
|---|---|---|---|---|---|---|
| _embedding rank_ | 0.561 | 0.552 | 0.588 | 0.511 | 0.632 | 0.391 |
| `contribution` | 0.738 | 0.818 | 0.688 | 0.736 | 0.660 | 0.671 |
| `role=background` | 0.336 | 0.294 | 0.451 | 0.442 | 0.456 | 0.437 |
| `role=final_answer` | 0.648 | 0.816 | 0.517 | 0.792 | 0.358 | 0.583 |
| `role=linking_fact` | 0.560 | 0.437 | 0.693 | 0.493 | 0.809 | 0.664 |
| `role=off_target` | 0.218 | 0.140 | 0.227 | 0.289 | 0.192 | 0.319 |
| `supports_a_step` | 0.763 | 0.830 | 0.721 | 0.661 | 0.755 | 0.691 |

## Combined score (logistic regression, trained on one dataset, scored on the other)

| combination | train koblex → musique gold vs hard-neg | train musique → koblex gold vs hard-neg | musique BRIDGE vs hard-neg (train koblex) |
|---|---|---|---|
| JEV answers | 0.756 | 0.843 | 0.779 |
| JEV answers + embedding rank | 0.813 | 0.878 | 0.837 |

## Mean answer by passage role

| feature | final | bridge | gold (statutes) | hard-neg | unanswerable-top |
|---|---|---|---|---|---|
| `contribution` | 2.55 | 2.24 | 3.24 | 2.05 | 1.79 |
| `role=background` | 0.08 | 0.09 | 0.12 | 0.19 | 0.12 |
| `role=final_answer` | 0.34 | 0.05 | 0.76 | 0.28 | 0.10 |
| `role=linking_fact` | 0.35 | 0.70 | 0.10 | 0.20 | 0.40 |
| `role=off_target` | 0.23 | 0.16 | 0.02 | 0.33 | 0.38 |
| `supports_a_step` | 0.60 | 0.67 | 0.86 | 0.52 | 0.46 |
