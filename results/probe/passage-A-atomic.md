# Probe report: passage-A-atomic

Literal atomic battery: answers / requested kind / same subject / links a query item / states information / instructs reader / contradicts premise. Domain-agnostic examples.

673 pairs · 673 paid calls · 0 cache hits · 1255018 input tokens (~US$0.053) · 20s

AUC (0.5 = no separation, 1 = perfect). Reference row: embedding rank alone.

| feature | all: gold vs hard-neg | koblex: gold vs hard-neg | musique: gold vs hard-neg | musique: FINAL vs hard-neg | musique: BRIDGE vs hard-neg | musique: gold vs unanswerable-top |
|---|---|---|---|---|---|---|
| _embedding rank_ | 0.561 | 0.552 | 0.588 | 0.511 | 0.632 | 0.391 |
| `answers_query` | 0.632 | 0.768 | 0.527 | 0.776 | 0.385 | 0.582 |
| `contradicts_query_premise` | 0.441 | 0.386 | 0.423 | 0.443 | 0.411 | 0.541 |
| `instructs_reader` | 0.500 | 0.473 | 0.433 | 0.471 | 0.410 | 0.368 |
| `links_query_item` | 0.635 | 0.665 | 0.597 | 0.681 | 0.549 | 0.631 |
| `same_subject` | 0.700 | 0.707 | 0.723 | 0.713 | 0.729 | 0.640 |
| `states_information` | 0.536 | 0.573 | 0.478 | 0.437 | 0.501 | 0.558 |
| `states_requested_kind` | 0.596 | 0.732 | 0.457 | 0.749 | 0.289 | 0.515 |

## Combined score (logistic regression, trained on one dataset, scored on the other)

| combination | train koblex → musique gold vs hard-neg | train musique → koblex gold vs hard-neg | musique BRIDGE vs hard-neg (train koblex) |
|---|---|---|---|
| JEV answers | 0.582 | 0.699 | 0.454 |
| JEV answers + embedding rank | 0.688 | 0.774 | 0.594 |

## Mean answer by passage role

| feature | final | bridge | gold (statutes) | hard-neg | unanswerable-top |
|---|---|---|---|---|---|
| `answers_query` | 0.34 | 0.08 | 0.40 | 0.17 | 0.11 |
| `contradicts_query_premise` | 0.21 | 0.23 | 0.13 | 0.19 | 0.20 |
| `instructs_reader` | 0.04 | 0.03 | 0.02 | 0.03 | 0.04 |
| `links_query_item` | 0.47 | 0.36 | 0.46 | 0.33 | 0.31 |
| `same_subject` | 0.57 | 0.58 | 0.67 | 0.46 | 0.45 |
| `states_information` | 0.97 | 0.98 | 0.98 | 0.97 | 0.96 |
| `states_requested_kind` | 0.73 | 0.20 | 0.74 | 0.50 | 0.36 |
