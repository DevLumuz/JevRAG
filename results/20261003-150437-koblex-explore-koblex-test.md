> **Partial run:** the JEV budget ran out; every system is reported on the first 142 questions (split order).

# Phase 2 — notebook loop vs. single pass (koblex, test split)

142 answerable questions · memory 44261 passages · loop: 3 rounds × 10 new passages, reads the 3 best per round, ≤ 2 facts per passage (P ≥ 0.50), notebook ≤ 8 facts.

Chain@10 = all gold passages of the question in the top 10. Found = all gold passages anywhere in what the system returns (all passages the loop saw).

| system | R@5 | R@10 | chain@5 | chain@10 | found (any rank) | JEV calls / q | US$ / q |
|---|---|---|---|---|---|---|---|
| vector, question only (option 1) | 0.789 | 0.854 | 0.613 | 0.718 | 0.859 | 0.0 | 0.0000 |
| notebook loop + JEV | 0.853 | 0.899 | 0.718 | 0.803 | 0.824 | 83.3 | 0.0000 |

US$/q counts only paid (uncached) tokens of this run.

## Paired differences, chain@10 (95% CI resampling questions)

- notebook loop + JEV − vector, question only (option 1): +0.085 [+0.028..+0.141]

## By number of hops (chain@10)

| system | 1 hops | 2 hops | 3 hops |
|---|---|---|---|
| vector, question only (option 1) | 0.95 (n=37) | 0.72 (n=79) | 0.38 (n=26) |
| notebook loop + JEV | 0.95 (n=37) | 0.81 (n=79) | 0.58 (n=26) |
