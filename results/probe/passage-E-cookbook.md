# Probe report: passage-E-cookbook

Control: the official TypeSafe 'classifying RAG passages' cookbook wording, unchanged.

673 pairs · 673 paid calls · 0 cache hits · 407711 input tokens (~US$0.017) · 20s

AUC (0.5 = no separation, 1 = perfect). Reference row: embedding rank alone.

| feature | all: gold vs hard-neg | koblex: gold vs hard-neg | musique: gold vs hard-neg | musique: FINAL vs hard-neg | musique: BRIDGE vs hard-neg | musique: gold vs unanswerable-top |
|---|---|---|---|---|---|---|
| _embedding rank_ | 0.561 | 0.552 | 0.588 | 0.511 | 0.632 | 0.391 |
| `contains_answer_evidence` | 0.714 | 0.828 | 0.631 | 0.760 | 0.557 | 0.678 |
| `is_relevant` | 0.688 | 0.826 | 0.633 | 0.802 | 0.536 | 0.647 |

## Combined score (logistic regression, trained on one dataset, scored on the other)

| combination | train koblex → musique gold vs hard-neg | train musique → koblex gold vs hard-neg | musique BRIDGE vs hard-neg (train koblex) |
|---|---|---|---|
| JEV answers | 0.640 | 0.830 | 0.558 |
| JEV answers + embedding rank | 0.709 | 0.867 | 0.635 |

## Mean answer by passage role

| feature | final | bridge | gold (statutes) | hard-neg | unanswerable-top |
|---|---|---|---|---|---|
| `contains_answer_evidence` | 0.60 | 0.38 | 0.85 | 0.46 | 0.31 |
| `is_relevant` | 0.47 | 0.25 | 0.80 | 0.39 | 0.22 |
