# Phase 3 — abstention from the notebook (musique-trainb, all)

300 questions: 150 answerable, 150 unanswerable (the memory lacks a needed passage). Paid JEV this run: ~US$0.711.

AUROC = probability that a random answerable question scores above a random unanswerable one (95% CI resampling questions). Balanced accuracy = mean of the hit rate on each class.

| signal | AUROC | best balanced acc. (threshold) | balanced acc. at --abstain-threshold |
|---|---|---|---|
| earlier gate (JEV on top-5 vector passages) | 0.830 [0.780–0.871] | 0.767 (0.33) | — |
| notebook: answer_stated | 0.817 [0.766–0.865] | 0.763 (0.28) | — |
| notebook: 1 − missing_link | 0.807 [0.755–0.853] | 0.743 (0.23) | — |
| notebook: coverage (mean of both) | 0.818 [0.766–0.865] | 0.747 (0.29) | — |
| mean(earlier gate, notebook coverage) — primary (frozen in plan §26) | 0.849 [0.802–0.888] | 0.790 (0.29) | 0.780 |
| loop: max answers_query over passages | 0.741 [0.683–0.793] | 0.687 (0.25) | — |
| earlier gate question on the loop's top 5 | 0.850 [0.803–0.890] | 0.783 (0.38) | — |
| coverage on notebook + loop's top 5 passages | 0.860 [0.814–0.900] | 0.787 (0.14) | — |
| mean(earlier gate on loop top 5, coverage + passages) | 0.861 [0.818–0.900] | 0.807 (0.25) | — |

Retrieval of the notebook loop on the 150 answerable questions: R@10 0.914 · chain@5 0.693 · chain@10 0.813.

Frozen threshold (--abstain-threshold) = 0.31, applied to the primary signal; it is chosen on dev only.
