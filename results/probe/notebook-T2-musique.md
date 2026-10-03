# P1 · T2 — does a notebook help JEV recognize the next link?

213 targets = gold passages of later reasoning steps, each with up to 4 non-gold passages from the top 30 of the notebook search (question + facts by vector, question + frontier entity by keywords). AUC within each target (1 = gold ranked above all its non-gold passages, 0.5 = chance), averaged; 95% CI resampling questions. Same passages in every condition; only the notebook view changes.

Questions: `next_needed` (is this passage a still-needed link?), `answers_query` (does it give the final answer, with the facts?), `max` = the larger of both.

## middle steps (n = 63)

| notebook in the state | `next_needed` | `answers_query` | `max` | `reranker` | `mean(max, reranker)` |
|---|---|---|---|---|---|
| _search-pool order (reference)_ | 0.817 [0.742–0.899] | | | | |
| no notebook (today) | 0.782 [0.708–0.853] | 0.579 [0.504–0.654] | 0.782 [0.708–0.853] | 0.552 [0.455–0.651] | 0.730 [0.652–0.806] |
| oracle facts + frontier | 0.901 [0.838–0.953] | 0.724 [0.650–0.792] | 0.901 [0.838–0.953] | 0.742 [0.646–0.828] | 0.893 [0.828–0.948] |
| oracle facts only | 0.893 [0.829–0.944] | 0.688 [0.605–0.762] | 0.893 [0.829–0.944] | 0.742 [0.646–0.828] | 0.877 [0.812–0.934] |
| wrong facts + frontier (other question) | 0.752 [0.663–0.830] | 0.575 [0.496–0.658] | 0.748 [0.658–0.826] | 0.571 [0.462–0.677] | 0.738 [0.652–0.817] |
| JEV-picked facts (T3), no frontier | 0.871 [0.808–0.927] | 0.687 [0.613–0.758] | 0.871 [0.808–0.927] | 0.714 [0.617–0.808] | 0.873 [0.806–0.933] |

Paired difference vs. no notebook (`next_needed`, `max`):

- oracle facts + frontier: `next_needed` +0.119 [+0.060..+0.179]; `max` +0.119 [+0.060..+0.179];
- oracle facts only: `next_needed` +0.111 [+0.047..+0.175]; `max` +0.111 [+0.047..+0.175];
- wrong facts + frontier (other question): `next_needed` -0.030 [-0.078..+0.020]; `max` -0.034 [-0.086..+0.018];
- JEV-picked facts (T3), no frontier: `next_needed` +0.089 [+0.031..+0.153]; `max` +0.089 [+0.031..+0.153];

Same condition, JEV `max` − reranker, and mean − reranker (Phase 1 gate: JEV must add ≥ 0.02 AUC over the reranker):

- no notebook (today): `max` +0.230 [+0.125..+0.331]; `mean(max, reranker)` +0.179 [+0.092..+0.263];
- oracle facts + frontier: `max` +0.159 [+0.069..+0.248]; `mean(max, reranker)` +0.151 [+0.076..+0.227];
- oracle facts only: `max` +0.151 [+0.059..+0.248]; `mean(max, reranker)` +0.135 [+0.068..+0.206];
- wrong facts + frontier (other question): `max` +0.177 [+0.069..+0.284]; `mean(max, reranker)` +0.167 [+0.073..+0.258];
- JEV-picked facts (T3), no frontier: `max` +0.157 [+0.054..+0.260]; `mean(max, reranker)` +0.159 [+0.070..+0.250];

## final steps (n = 150)

| notebook in the state | `next_needed` | `answers_query` | `max` | `reranker` | `mean(max, reranker)` |
|---|---|---|---|---|---|
| _search-pool order (reference)_ | 0.933 [0.903–0.958] | | | | |
| no notebook (today) | 0.897 [0.862–0.929] | 0.939 [0.915–0.961] | 0.895 [0.860–0.927] | 0.835 [0.790–0.878] | 0.905 [0.870–0.938] |
| oracle facts + frontier | 0.997 [0.993–1.000] | 0.984 [0.969–0.995] | 0.993 [0.985–0.999] | 0.897 [0.860–0.928] | 0.977 [0.958–0.992] |
| oracle facts only | 0.988 [0.977–0.997] | 0.978 [0.959–0.993] | 0.985 [0.972–0.997] | 0.897 [0.860–0.928] | 0.975 [0.957–0.990] |
| wrong facts + frontier (other question) | 0.898 [0.861–0.932] | 0.927 [0.899–0.952] | 0.898 [0.861–0.932] | 0.792 [0.745–0.838] | 0.902 [0.868–0.932] |
| JEV-picked facts (T3), no frontier | 0.974 [0.958–0.988] | 0.978 [0.960–0.992] | 0.971 [0.953–0.985] | 0.878 [0.835–0.920] | 0.960 [0.940–0.978] |

Paired difference vs. no notebook (`next_needed`, `max`):

- oracle facts + frontier: `next_needed` +0.100 [+0.068..+0.133]; `max` +0.098 [+0.068..+0.132];
- oracle facts only: `next_needed` +0.092 [+0.060..+0.128]; `max` +0.090 [+0.058..+0.124];
- wrong facts + frontier (other question): `next_needed` +0.002 [-0.019..+0.022]; `max` +0.003 [-0.017..+0.022];
- JEV-picked facts (T3), no frontier: `next_needed` +0.077 [+0.047..+0.109]; `max` +0.076 [+0.046..+0.107];

Same condition, JEV `max` − reranker, and mean − reranker (Phase 1 gate: JEV must add ≥ 0.02 AUC over the reranker):

- no notebook (today): `max` +0.060 [+0.015..+0.105]; `mean(max, reranker)` +0.070 [+0.035..+0.108];
- oracle facts + frontier: `max` +0.097 [+0.062..+0.133]; `mean(max, reranker)` +0.080 [+0.052..+0.112];
- oracle facts only: `max` +0.088 [+0.057..+0.123]; `mean(max, reranker)` +0.078 [+0.050..+0.110];
- wrong facts + frontier (other question): `max` +0.107 [+0.058..+0.152]; `mean(max, reranker)` +0.110 [+0.068..+0.150];
- JEV-picked facts (T3), no frontier: `max` +0.092 [+0.050..+0.134]; `mean(max, reranker)` +0.082 [+0.045..+0.118];

## all later steps (n = 213)

| notebook in the state | `next_needed` | `answers_query` | `max` | `reranker` | `mean(max, reranker)` |
|---|---|---|---|---|---|
| _search-pool order (reference)_ | 0.899 [0.868–0.929] | | | | |
| no notebook (today) | 0.863 [0.829–0.895] | 0.833 [0.800–0.865] | 0.862 [0.828–0.895] | 0.751 [0.704–0.801] | 0.853 [0.821–0.887] |
| oracle facts + frontier | 0.968 [0.949–0.985] | 0.907 [0.879–0.934] | 0.966 [0.946–0.983] | 0.851 [0.811–0.889] | 0.952 [0.928–0.972] |
| oracle facts only | 0.960 [0.941–0.978] | 0.893 [0.859–0.924] | 0.958 [0.938–0.976] | 0.851 [0.811–0.889] | 0.946 [0.923–0.966] |
| wrong facts + frontier (other question) | 0.855 [0.818–0.891] | 0.823 [0.785–0.861] | 0.854 [0.817–0.889] | 0.727 [0.679–0.774] | 0.853 [0.820–0.887] |
| JEV-picked facts (T3), no frontier | 0.944 [0.923–0.965] | 0.892 [0.864–0.920] | 0.941 [0.919–0.963] | 0.830 [0.783–0.874] | 0.934 [0.911–0.957] |

Paired difference vs. no notebook (`next_needed`, `max`):

- oracle facts + frontier: `next_needed` +0.106 [+0.077..+0.136]; `max` +0.104 [+0.076..+0.132];
- oracle facts only: `next_needed` +0.097 [+0.067..+0.129]; `max` +0.096 [+0.067..+0.127];
- wrong facts + frontier (other question): `next_needed` -0.008 [-0.029..+0.012]; `max` -0.008 [-0.030..+0.013];
- JEV-picked facts (T3), no frontier: `next_needed` +0.081 [+0.050..+0.112]; `max` +0.080 [+0.051..+0.110];

Same condition, JEV `max` − reranker, and mean − reranker (Phase 1 gate: JEV must add ≥ 0.02 AUC over the reranker):

- no notebook (today): `max` +0.110 [+0.062..+0.156]; `mean(max, reranker)` +0.102 [+0.064..+0.141];
- oracle facts + frontier: `max` +0.115 [+0.076..+0.156]; `mean(max, reranker)` +0.101 [+0.069..+0.134];
- oracle facts only: `max` +0.107 [+0.067..+0.147]; `mean(max, reranker)` +0.095 [+0.066..+0.126];
- wrong facts + frontier (other question): `max` +0.127 [+0.079..+0.173]; `mean(max, reranker)` +0.127 [+0.085..+0.167];
- JEV-picked facts (T3), no frontier: `max` +0.112 [+0.067..+0.161]; `mean(max, reranker)` +0.104 [+0.066..+0.145];

Gate (plan §22.3 P1): oracle raises middle-step AUC by ≥ 0.10 and wrong facts lower it by no more than 0.05.

---
0 paid calls · 5325 cache hits · 0 input tokens (~US$0.000)
