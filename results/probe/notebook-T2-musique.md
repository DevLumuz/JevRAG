# P1 · T2 — does a notebook help JEV recognize the next link?

213 targets = gold passages of later reasoning steps, each with up to 4 non-gold passages from the top 30 of the notebook search (question + facts by vector, question + frontier entity by keywords). AUC within each target (1 = gold ranked above all its non-gold passages, 0.5 = chance), averaged; 95% CI resampling questions. Same passages in every condition; only the notebook view changes.

Questions: `next_needed` (is this passage a still-needed link?), `answers_query` (does it give the final answer, with the facts?), `max` = the larger of both.

## middle steps (n = 63)

| notebook in the state | `next_needed` | `answers_query` | `max` |
|---|---|---|---|
| _search-pool order (reference)_ | 0.817 [0.742–0.899] | | |
| no notebook (today) | 0.786 [0.713–0.855] | 0.583 [0.510–0.658] | 0.786 [0.713–0.855] |
| oracle facts + frontier | 0.901 [0.838–0.953] | 0.724 [0.650–0.792] | 0.901 [0.838–0.953] |
| oracle facts only | 0.893 [0.829–0.944] | 0.688 [0.605–0.762] | 0.893 [0.829–0.944] |
| wrong facts + frontier (other question) | 0.752 [0.663–0.830] | 0.573 [0.493–0.658] | 0.748 [0.658–0.826] |
| JEV-picked facts (T3), no frontier | 0.871 [0.808–0.927] | 0.687 [0.613–0.758] | 0.871 [0.808–0.927] |

Paired difference vs. no notebook (`next_needed`, `max`):

- oracle facts + frontier: `next_needed` +0.115 [+0.050..+0.177]; `max` +0.115 [+0.050..+0.177];
- oracle facts only: `next_needed` +0.107 [+0.038..+0.175]; `max` +0.107 [+0.038..+0.175];
- wrong facts + frontier (other question): `next_needed` -0.034 [-0.088..+0.019]; `max` -0.038 [-0.095..+0.018];
- JEV-picked facts (T3), no frontier: `next_needed` +0.085 [+0.022..+0.150]; `max` +0.085 [+0.022..+0.150];

## final steps (n = 150)

| notebook in the state | `next_needed` | `answers_query` | `max` |
|---|---|---|---|
| _search-pool order (reference)_ | 0.933 [0.903–0.958] | | |
| no notebook (today) | 0.897 [0.862–0.929] | 0.939 [0.915–0.961] | 0.895 [0.860–0.927] |
| oracle facts + frontier | 0.997 [0.993–1.000] | 0.984 [0.969–0.995] | 0.993 [0.985–0.999] |
| oracle facts only | 0.988 [0.977–0.997] | 0.978 [0.959–0.993] | 0.985 [0.972–0.997] |
| wrong facts + frontier (other question) | 0.898 [0.861–0.932] | 0.927 [0.899–0.952] | 0.898 [0.861–0.932] |
| JEV-picked facts (T3), no frontier | 0.974 [0.958–0.988] | 0.978 [0.960–0.992] | 0.971 [0.953–0.985] |

Paired difference vs. no notebook (`next_needed`, `max`):

- oracle facts + frontier: `next_needed` +0.100 [+0.068..+0.133]; `max` +0.098 [+0.068..+0.132];
- oracle facts only: `next_needed` +0.092 [+0.060..+0.128]; `max` +0.090 [+0.058..+0.124];
- wrong facts + frontier (other question): `next_needed` +0.002 [-0.019..+0.022]; `max` +0.003 [-0.017..+0.022];
- JEV-picked facts (T3), no frontier: `next_needed` +0.077 [+0.047..+0.109]; `max` +0.076 [+0.046..+0.107];

## all later steps (n = 213)

| notebook in the state | `next_needed` | `answers_query` | `max` |
|---|---|---|---|
| _search-pool order (reference)_ | 0.899 [0.868–0.929] | | |
| no notebook (today) | 0.864 [0.830–0.897] | 0.834 [0.802–0.867] | 0.863 [0.829–0.896] |
| oracle facts + frontier | 0.968 [0.949–0.985] | 0.907 [0.879–0.934] | 0.966 [0.946–0.983] |
| oracle facts only | 0.960 [0.941–0.978] | 0.893 [0.859–0.924] | 0.958 [0.938–0.976] |
| wrong facts + frontier (other question) | 0.855 [0.818–0.891] | 0.822 [0.785–0.861] | 0.854 [0.817–0.889] |
| JEV-picked facts (T3), no frontier | 0.944 [0.923–0.965] | 0.892 [0.864–0.920] | 0.941 [0.919–0.963] |

Paired difference vs. no notebook (`next_needed`, `max`):

- oracle facts + frontier: `next_needed` +0.104 [+0.075..+0.136]; `max` +0.103 [+0.074..+0.131];
- oracle facts only: `next_needed` +0.096 [+0.065..+0.129]; `max` +0.095 [+0.065..+0.127];
- wrong facts + frontier (other question): `next_needed` -0.009 [-0.032..+0.012]; `max` -0.009 [-0.032..+0.012];
- JEV-picked facts (T3), no frontier: `next_needed` +0.080 [+0.049..+0.111]; `max` +0.079 [+0.049..+0.109];

Gate (plan §22.3 P1): oracle raises middle-step AUC by ≥ 0.10 and wrong facts lower it by no more than 0.05.

---
4395 paid calls · 930 cache hits · 3786324 input tokens (~US$0.159)
