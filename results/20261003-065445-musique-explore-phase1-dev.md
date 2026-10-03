# Phase 2 — notebook loop vs. single pass (musique, dev split)

34 answerable questions · memory 4757 passages · loop: 3 rounds × 10 new passages, reads the 3 best per round, ≤ 2 facts per passage (P ≥ 0.50), notebook ≤ 8 facts.

Chain@10 = all gold passages of the question in the top 10. Found = all gold passages anywhere in what the system returns (all passages the loop saw).

| system | R@5 | R@10 | chain@5 | chain@10 | found (any rank) | JEV calls / q | US$ / q |
|---|---|---|---|---|---|---|---|
| vector, question only (option 1) | 0.738 | 0.819 | 0.412 | 0.588 | 0.706 | 0.0 | 0.0000 |
| single pass + JEV scores 30 | 0.713 | 0.806 | 0.412 | 0.559 | 0.706 | 30.0 | 0.0000 |
| single pass + reranker scores 30 | 0.625 | 0.725 | 0.265 | 0.441 | 0.706 | 0.0 | 0.0000 |
| single pass + reranker scores 60 | 0.596 | 0.645 | 0.176 | 0.265 | 0.853 | 0.0 | 0.0000 |
| single pass + JEV & reranker mean, 30 | 0.723 | 0.806 | 0.441 | 0.588 | 0.706 | 30.0 | 0.0000 |
| notebook loop · reranker scores, reranker picks sentences (no JEV) | 0.331 | 0.676 | 0.059 | 0.324 | 0.765 | 0.0 | 0.0000 |
| notebook loop · reranker scores, JEV picks sentences | 0.608 | 0.647 | 0.235 | 0.294 | 0.794 | 31.6 | 0.0006 |
| notebook loop · JEV & reranker mean scores, JEV picks sentences | 0.730 | 0.826 | 0.441 | 0.559 | 0.824 | 63.4 | 0.0008 |
| notebook loop + JEV | 0.772 | 0.926 | 0.529 | 0.794 | 0.882 | 66.3 | 0.0000 |

US$/q counts only paid (uncached) tokens of this run.

## Paired differences, chain@10 (95% CI resampling questions)

- notebook loop + JEV − vector, question only (option 1): +0.206 [+0.029..+0.382]
- notebook loop + JEV − single pass + JEV scores 30: +0.235 [+0.088..+0.382]
- notebook loop + JEV − single pass + reranker scores 30: +0.353 [+0.206..+0.529]
- notebook loop + JEV − single pass + reranker scores 60: +0.529 [+0.353..+0.706]
- notebook loop + JEV − single pass + JEV & reranker mean, 30: +0.206 [+0.088..+0.353]
- notebook loop + JEV − notebook loop · reranker scores, reranker picks sentences (no JEV): +0.471 [+0.324..+0.647]
- notebook loop + JEV − notebook loop · reranker scores, JEV picks sentences: +0.500 [+0.324..+0.676]
- notebook loop + JEV − notebook loop · JEV & reranker mean scores, JEV picks sentences: +0.235 [+0.118..+0.382]

## By number of hops (chain@10)

| system | 2 hops | 3 hops | 4 hops |
|---|---|---|---|
| vector, question only (option 1) | 0.79 (n=14) | 0.53 (n=15) | 0.20 (n=5) |
| single pass + JEV scores 30 | 0.93 (n=14) | 0.40 (n=15) | 0.00 (n=5) |
| single pass + reranker scores 30 | 0.64 (n=14) | 0.40 (n=15) | 0.00 (n=5) |
| single pass + reranker scores 60 | 0.43 (n=14) | 0.20 (n=15) | 0.00 (n=5) |
| single pass + JEV & reranker mean, 30 | 0.93 (n=14) | 0.47 (n=15) | 0.00 (n=5) |
| notebook loop · reranker scores, reranker picks sentences (no JEV) | 0.64 (n=14) | 0.13 (n=15) | 0.00 (n=5) |
| notebook loop · reranker scores, JEV picks sentences | 0.57 (n=14) | 0.13 (n=15) | 0.00 (n=5) |
| notebook loop · JEV & reranker mean scores, JEV picks sentences | 0.93 (n=14) | 0.33 (n=15) | 0.20 (n=5) |
| notebook loop + JEV | 1.00 (n=14) | 0.80 (n=15) | 0.20 (n=5) |
