# Phase 2 — notebook loop vs. single pass (musique-train, all split)

150 answerable questions · memory 3402 passages · loop: 3 rounds × 10 new passages, reads the 3 best per round, ≤ 2 facts per passage (P ≥ 0.50), notebook ≤ 8 facts.

Chain@10 = all gold passages of the question in the top 10. Found = all gold passages anywhere in what the system returns (all passages the loop saw).

| system | R@5 | R@10 | chain@5 | chain@10 | found (any rank) | JEV calls / q | US$ / q |
|---|---|---|---|---|---|---|---|
| vector, question only (option 1) | 0.817 | 0.876 | 0.640 | 0.727 | 0.833 | 0.0 | 0.0000 |
| notebook loop, no JEV (word-overlap rules) | 0.764 | 0.826 | 0.567 | 0.673 | 0.853 | 0.0 | 0.0000 |
| vector single pass + JEV scores 30 | 0.806 | 0.890 | 0.633 | 0.767 | 0.833 | 30.0 | 0.0010 |
| vector single pass + JEV scores 60 (equal budget) | 0.796 | 0.883 | 0.620 | 0.753 | 0.887 | 60.0 | 0.0010 |
| notebook loop + JEV | 0.854 | 0.926 | 0.700 | 0.827 | 0.867 | 65.9 | 0.0018 |

US$/q counts only paid (uncached) tokens of this run.

## Paired differences, chain@10 (95% CI resampling questions)

- notebook loop + JEV − vector, question only (option 1): +0.100 [+0.047..+0.160]
- notebook loop + JEV − notebook loop, no JEV (word-overlap rules): +0.153 [+0.087..+0.227]
- notebook loop + JEV − vector single pass + JEV scores 30: +0.060 [+0.020..+0.107]
- notebook loop + JEV − vector single pass + JEV scores 60 (equal budget): +0.073 [+0.033..+0.120]

## By number of hops (chain@10)

| system | 2 hops | 3 hops | 4 hops |
|---|---|---|---|
| vector, question only (option 1) | 0.88 (n=112) | 0.35 (n=26) | 0.17 (n=12) |
| notebook loop, no JEV (word-overlap rules) | 0.82 (n=112) | 0.23 (n=26) | 0.25 (n=12) |
| vector single pass + JEV scores 30 | 0.89 (n=112) | 0.42 (n=26) | 0.33 (n=12) |
| vector single pass + JEV scores 60 (equal budget) | 0.88 (n=112) | 0.38 (n=26) | 0.33 (n=12) |
| notebook loop + JEV | 0.95 (n=112) | 0.46 (n=26) | 0.50 (n=12) |
