# P1 · T3 — can JEV pick the key sentence of a gold passage?

386 gold passages (one per reasoning step), split into sentences; the state carries the question and the oracle facts of earlier steps. Target = the silver sentence (contains the step's answer). top-1 = the highest-scored sentence is the silver one; top-2 = it is in the two best. JEV choice also has a last option "none"; it is reported apart.

Mean sentences per passage: 3.6 (random pick top-1 ≈ 0.38). Silver sentence is the first one in 262 of 386 passages.

## all steps (n = 386)

| method | top-1 | top-2 |
|---|---|---|
| first sentence (baseline) | 0.68 [0.64–0.72] | 0.73 [0.68–0.77] |
| word overlap with question (baseline) | 0.60 [0.55–0.65] | 0.81 [0.77–0.84] |
| word overlap with question + known facts (baseline) | 0.66 [0.61–0.71] | 0.84 [0.81–0.88] |
| JEV choice over sentences | 0.89 [0.86–0.92] | 0.97 [0.95–0.98] |
| JEV yes/no per sentence | 0.87 [0.83–0.90] | 0.97 [0.95–0.98] |

## first steps (n = 173)

| method | top-1 | top-2 |
|---|---|---|
| first sentence (baseline) | 0.77 [0.70–0.83] | 0.80 [0.73–0.86] |
| word overlap with question (baseline) | 0.66 [0.60–0.73] | 0.85 [0.80–0.90] |
| word overlap with question + known facts (baseline) | 0.66 [0.60–0.73] | 0.85 [0.80–0.90] |
| JEV choice over sentences | 0.91 [0.86–0.95] | 0.98 [0.95–0.99] |
| JEV yes/no per sentence | 0.90 [0.85–0.94] | 0.98 [0.95–0.99] |

## middle steps (n = 63)

| method | top-1 | top-2 |
|---|---|---|
| first sentence (baseline) | 0.56 [0.44–0.66] | 0.60 [0.50–0.71] |
| word overlap with question (baseline) | 0.43 [0.31–0.55] | 0.65 [0.53–0.77] |
| word overlap with question + known facts (baseline) | 0.54 [0.42–0.66] | 0.81 [0.71–0.90] |
| JEV choice over sentences | 0.79 [0.70–0.89] | 0.92 [0.85–0.98] |
| JEV yes/no per sentence | 0.75 [0.65–0.84] | 0.97 [0.92–1.00] |

## final steps (n = 150)

| method | top-1 | top-2 |
|---|---|---|
| first sentence (baseline) | 0.63 [0.55–0.70] | 0.69 [0.62–0.77] |
| word overlap with question (baseline) | 0.59 [0.51–0.67] | 0.83 [0.77–0.89] |
| word overlap with question + known facts (baseline) | 0.70 [0.63–0.77] | 0.85 [0.79–0.91] |
| JEV choice over sentences | 0.91 [0.85–0.95] | 0.97 [0.95–0.99] |
| JEV yes/no per sentence | 0.88 [0.83–0.93] | 0.96 [0.93–0.99] |

## passages with ≥ 3 sentences, silver not first (n = 106)

| method | top-1 | top-2 |
|---|---|---|
| first sentence (baseline) | 0.00 [0.00–0.00] | 0.00 [0.00–0.00] |
| word overlap with question (baseline) | 0.43 [0.34–0.53] | 0.62 [0.54–0.70] |
| word overlap with question + known facts (baseline) | 0.44 [0.36–0.53] | 0.65 [0.58–0.73] |
| JEV choice over sentences | 0.84 [0.76–0.91] | 0.92 [0.87–0.97] |
| JEV yes/no per sentence | 0.80 [0.72–0.88] | 0.94 [0.90–0.98] |

JEV choice answered "none" in 70 of 386 passages (all contain the step's fact). Mean confidence 0.73.

---
1763 paid calls · 14 cache hits · 1111179 input tokens (~US$0.047)
