# Phase 2 — notebook loop vs. single pass (musique-train, all split)

150 answerable questions · memory 3402 passages · loop: 3 rounds × 10 new passages, reads the 3 best per round, ≤ 2 facts per passage (P ≥ 0.50), notebook ≤ 8 facts.

Chain@10 = all gold passages of the question in the top 10. Found = all gold passages anywhere in what the system returns (all passages the loop saw).

| system | R@5 | R@10 | chain@5 | chain@10 | found (any rank) | JEV calls / q | US$ / q |
|---|---|---|---|---|---|---|---|
| vector, question only (option 1) | 0.817 | 0.876 | 0.640 | 0.727 | 0.833 | 0.0 | 0.0000 |
| single pass + reranker scores 30 | 0.730 | 0.836 | 0.467 | 0.627 | 0.833 | 0.0 | 0.0000 |
| single pass + JEV & reranker mean, 30 | 0.821 | 0.905 | 0.613 | 0.780 | 0.833 | 30.0 | 0.0000 |
| single pass + JEV scores 30 | 0.806 | 0.890 | 0.633 | 0.767 | 0.833 | 30.0 | 0.0000 |
| notebook loop + JEV | 0.854 | 0.926 | 0.700 | 0.827 | 0.867 | 65.9 | 0.0000 |

US$/q counts only paid (uncached) tokens of this run.

## Paired differences, chain@10 (95% CI resampling questions)

- notebook loop + JEV − vector, question only (option 1): +0.100 [+0.047..+0.160]
- notebook loop + JEV − single pass + reranker scores 30: +0.200 [+0.127..+0.280]
- notebook loop + JEV − single pass + JEV & reranker mean, 30: +0.047 [+0.007..+0.093]
- notebook loop + JEV − single pass + JEV scores 30: +0.060 [+0.020..+0.107]

## By number of hops (chain@10)

| system | 2 hops | 3 hops | 4 hops |
|---|---|---|---|
| vector, question only (option 1) | 0.88 (n=112) | 0.35 (n=26) | 0.17 (n=12) |
| single pass + reranker scores 30 | 0.79 (n=112) | 0.15 (n=26) | 0.08 (n=12) |
| single pass + JEV & reranker mean, 30 | 0.91 (n=112) | 0.42 (n=26) | 0.33 (n=12) |
| single pass + JEV scores 30 | 0.89 (n=112) | 0.42 (n=26) | 0.33 (n=12) |
| notebook loop + JEV | 0.95 (n=112) | 0.46 (n=26) | 0.50 (n=12) |
