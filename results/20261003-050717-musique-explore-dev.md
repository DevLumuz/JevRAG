# Phase 2 — notebook loop vs. single pass (musique, dev split)

34 answerable questions · memory 4757 passages · loop: 3 rounds × 10 new passages, reads the 3 best per round, ≤ 2 facts per passage (P ≥ 0.50), notebook ≤ 8 facts.

Chain@10 = all gold passages of the question in the top 10. Found = all gold passages anywhere in what the system returns (all passages the loop saw).

| system | R@5 | R@10 | chain@5 | chain@10 | found (any rank) | JEV calls / q | US$ / q |
|---|---|---|---|---|---|---|---|
| vector, question only (option 1) | 0.738 | 0.819 | 0.412 | 0.588 | 0.706 | 0.0 | 0.0000 |
| notebook loop, no JEV (word-overlap rules) | 0.716 | 0.777 | 0.382 | 0.500 | 0.794 | 0.0 | 0.0000 |
| vector single pass + JEV scores 30 | 0.713 | 0.806 | 0.412 | 0.559 | 0.706 | 30.0 | 0.0006 |
| vector single pass + JEV scores 60 (equal budget) | 0.703 | 0.794 | 0.412 | 0.559 | 0.853 | 60.0 | 0.0007 |
| notebook loop + JEV | 0.772 | 0.926 | 0.529 | 0.794 | 0.882 | 66.3 | 0.0013 |

US$/q counts only paid (uncached) tokens of this run.

## Paired differences, chain@10 (95% CI resampling questions)

- notebook loop + JEV − vector, question only (option 1): +0.206 [+0.029..+0.382]
- notebook loop + JEV − notebook loop, no JEV (word-overlap rules): +0.294 [+0.118..+0.471]
- notebook loop + JEV − vector single pass + JEV scores 30: +0.235 [+0.088..+0.382]
- notebook loop + JEV − vector single pass + JEV scores 60 (equal budget): +0.235 [+0.088..+0.382]

## By number of hops (chain@10)

| system | 2 hops | 3 hops | 4 hops |
|---|---|---|---|
| vector, question only (option 1) | 0.79 (n=14) | 0.53 (n=15) | 0.20 (n=5) |
| notebook loop, no JEV (word-overlap rules) | 0.79 (n=14) | 0.40 (n=15) | 0.00 (n=5) |
| vector single pass + JEV scores 30 | 0.93 (n=14) | 0.40 (n=15) | 0.00 (n=5) |
| vector single pass + JEV scores 60 (equal budget) | 0.93 (n=14) | 0.40 (n=15) | 0.00 (n=5) |
| notebook loop + JEV | 1.00 (n=14) | 0.80 (n=15) | 0.20 (n=5) |
