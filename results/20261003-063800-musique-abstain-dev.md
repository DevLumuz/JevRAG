# Phase 3 — abstention from the notebook (musique, dev)

82 questions: 34 answerable, 48 unanswerable (the memory lacks a needed passage). Paid JEV this run: ~US$0.011.

AUROC = probability that a random answerable question scores above a random unanswerable one (95% CI resampling questions). Balanced accuracy = mean of the hit rate on each class.

| signal | AUROC | best balanced acc. (threshold) | balanced acc. at --abstain-threshold |
|---|---|---|---|
| earlier gate (JEV on top-5 vector passages) | 0.771 [0.662–0.865] | 0.711 (0.28) | — |
| notebook: answer_stated | 0.765 [0.654–0.876] | 0.730 (0.24) | — |
| notebook: 1 − missing_link | 0.767 [0.657–0.875] | 0.732 (0.21) | — |
| notebook: coverage (mean of both) — primary | 0.767 [0.656–0.874] | 0.732 (0.15) | 0.681 |
| loop: max answers_query over passages | 0.690 [0.578–0.795] | 0.680 (0.28) | — |
| earlier gate question on the loop's top 5 | 0.726 [0.614–0.826] | 0.678 (0.19) | — |
| coverage on notebook + loop's top 5 passages | 0.738 [0.621–0.852] | 0.707 (0.20) | — |
| mean(earlier gate on loop top 5, coverage + passages) | 0.752 [0.645–0.851] | 0.684 (0.31) | — |

Frozen threshold (--abstain-threshold) = 0.50, applied to the primary signal (notebook coverage); it is chosen on dev only.
