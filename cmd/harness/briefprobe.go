package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"jev/internal/brief"
	"jev/internal/llm"
	"jev/internal/probe"
	"jev/internal/retrieval"
)

// Gemini 3.8 Flash list price (US$ per million tokens), through 2026.
const (
	briefInPrice  = 0.75
	briefOutPrice = 3.75
)

// runBriefProbe measures what a search brief written by the calling agent
// (simulated with Gemini) adds to retrieval, before any JEV call: for each
// channel of the brief, how often the gold passages reach the top 10 / 30
// (the pool JEV judges). No JEV is spent.
func runBriefProbe(ctx context.Context, cfg config, bench *benchmark, qs []benchQuestion, qEmb map[string][]float64, querySpace *embedSpace) error {
	var sel []benchQuestion
	for _, q := range qs {
		if q.Answerable {
			sel = append(sel, q)
		}
	}
	// Hard multi-hop questions first (all of them), then the rest, up to --brief-n.
	sort.SliceStable(sel, func(i, j int) bool { return sel[i].Hops > sel[j].Hops })
	if cfg.briefN > 0 && len(sel) > cfg.briefN {
		sel = sel[:cfg.briefN]
	}
	gen, err := llm.NewGemini(ctx, llm.Options{Model: cfg.llmModel})
	if err != nil {
		return err
	}
	bm := retrieval.NewBM25(bench.Nodes, bench.DocText)

	var report strings.Builder
	fmt.Fprintf(&report, "# Search brief probe — %s (%d questions)\n\n", bench.Name, len(sel))
	report.WriteString("The calling agent is simulated with Gemini. Reach@K = share of gold passages in the top K of that channel; chain@K = questions with every gold passage in the top K (30 = the pool JEV judges). No JEV calls.\n\n")
	var spentUSD float64
	for _, variant := range strings.Split(cfg.briefVariants, ",") {
		path := filepath.Join("data", "briefs", fmt.Sprintf("%s-%s.json", bench.Name, variant))
		briefs := map[string]*brief.Brief{}
		if _, err := readJSON(path, &briefs); err != nil {
			return err
		}
		var in, out int64
		for i, q := range sel {
			if briefs[q.ID] != nil {
				continue
			}
			if spentUSD+usd(in, out) > cfg.briefMaxUSD {
				return fmt.Errorf("brief budget reached (~US$%.3f); %d briefs done", spentUSD+usd(in, out), i)
			}
			text, ti, to, err := gen.GenerateJSON(ctx, brief.Prompt(q.Text, variant), brief.Schema)
			in, out = in+ti, out+to
			if err != nil {
				return fmt.Errorf("%s: %w", q.ID, err)
			}
			b, err := brief.Parse(text)
			if err != nil {
				return fmt.Errorf("%s: %w", q.ID, err)
			}
			briefs[q.ID] = b
			if err := writeJSON(path, briefs); err != nil {
				return err
			}
		}
		spentUSD += usd(in, out)
		log.Printf("briefs %s: %d new Gemini tokens in/out %d/%d (~US$%.3f)", variant, len(sel), in, out, usd(in, out))

		// Embed every query text the channels need (cached by content).
		var texts []string
		for _, q := range sel {
			b := briefs[q.ID]
			texts = append(texts, b.Question, b.Hypothetical)
			for _, s := range b.Steps {
				texts = append(texts, brief.SearchText(s.Ask))
			}
		}
		vecs, err := embedAll(ctx, cfg, []embedJob{{name: "brief-" + variant, space: querySpace, texts: texts}})
		if err != nil {
			return err
		}
		red, err := reduceAll(vecs[0], cfg.dims)
		if err != nil {
			return err
		}
		emb := map[string][]float64{}
		for i, t := range texts {
			emb[t] = red[i]
		}

		const depth = 300
		vec := func(e []float64) []retrieval.Scored { return retrieval.VectorSearch(e, bench.Nodes, depth) }
		rrf := func(ls ...[]retrieval.Scored) []retrieval.Scored { return retrieval.FuseRRF(60, depth, ls...) }
		steps := func(b *brief.Brief) [][]retrieval.Scored {
			var ls [][]retrieval.Scored
			for _, s := range b.Steps {
				ls = append(ls, vec(emb[brief.SearchText(s.Ask)]))
			}
			return ls
		}
		channels := []struct {
			name string
			rank func(q benchQuestion, b *brief.Brief) []retrieval.Scored
		}{
			{"question alone (today)", func(q benchQuestion, _ *brief.Brief) []retrieval.Scored { return vec(qEmb[q.ID]) }},
			{"agent's self-contained question", func(_ benchQuestion, b *brief.Brief) []retrieval.Scored { return vec(emb[b.Question]) }},
			{"steps (one search per step, fused)", func(_ benchQuestion, b *brief.Brief) []retrieval.Scored { return rrf(steps(b)...) }},
			{"terms (keywords)", func(_ benchQuestion, b *brief.Brief) []retrieval.Scored {
				return bm.Search(strings.Join(b.Terms, " "), depth)
			}},
			{"hypothetical passage", func(_ benchQuestion, b *brief.Brief) []retrieval.Scored { return vec(emb[b.Hypothetical]) }},
			{"question + steps", func(q benchQuestion, b *brief.Brief) []retrieval.Scored {
				return rrf(append([][]retrieval.Scored{vec(qEmb[q.ID])}, steps(b)...)...)
			}},
			{"question + terms", func(q benchQuestion, b *brief.Brief) []retrieval.Scored {
				return rrf(vec(qEmb[q.ID]), bm.Search(strings.Join(b.Terms, " "), depth))
			}},
			{"all channels fused", func(q benchQuestion, b *brief.Brief) []retrieval.Scored {
				ls := append([][]retrieval.Scored{vec(qEmb[q.ID]), vec(emb[b.Question]), vec(emb[b.Hypothetical]),
					bm.Search(strings.Join(b.Terms, " "), depth)}, steps(b)...)
				return rrf(ls...)
			}},
		}
		fmt.Fprintf(&report, "## Variant: %s\n\n", variant)
		if variant == brief.Blind {
			report.WriteString("The agent was told the documents are private and must not supply names, dates or answers (closest to company documents).\n\n")
		} else {
			report.WriteString("The agent may use its own general knowledge (it may already know public facts).\n\n")
		}
		report.WriteString("| channel | reach@10 | reach@30 | chain@10 | chain@30 |\n|---|---|---|---|---|\n")
		type acc struct{ r10, r30, c10, c30 []float64 }
		for _, ch := range channels {
			var a acc
			var cl []string
			for _, q := range sel {
				keys := make([]string, 0, depth)
				for _, s := range ch.rank(q, briefs[q.ID]) {
					keys = append(keys, s.Node.Key)
				}
				got := bench.Canon(keys)
				r10, r30 := recallAt(q.Gold, got, 10), recallAt(q.Gold, got, 30)
				a.r10, a.r30 = append(a.r10, r10), append(a.r30, r30)
				a.c10, a.c30 = append(a.c10, b2f(r10 == 1)), append(a.c30, b2f(r30 == 1))
				cl = append(cl, q.ID)
			}
			cell := func(v []float64, seed uint64) string {
				m, lo, hi := probe.ClusterMean(v, cl, 2000, seed)
				return fmt.Sprintf("%.3f [%.2f–%.2f]", m, lo, hi)
			}
			fmt.Fprintf(&report, "| %s | %s | %s | %s | %s |\n", ch.name, cell(a.r10, 1), cell(a.r30, 2), cell(a.c10, 3), cell(a.c30, 4))
		}
		report.WriteString("\n")
	}
	fmt.Fprintf(&report, "Gemini spent on new briefs this run: ~US$%.3f.\n", spentUSD)
	fmt.Print(report.String())
	out := filepath.Join(cfg.resultsDir, "probe", fmt.Sprintf("brief-%s.md", bench.Name))
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, []byte(report.String()), 0o644); err != nil {
		return err
	}
	log.Printf("report: %s", out)
	return nil
}

func usd(in, out int64) float64 {
	return float64(in)/1e6*briefInPrice + float64(out)/1e6*briefOutPrice
}

func recallAt(gold, got []string, k int) float64 {
	if len(gold) == 0 {
		return 0
	}
	top := map[string]bool{}
	for _, g := range got[:min(k, len(got))] {
		top[g] = true
	}
	n := 0
	for _, g := range gold {
		if top[g] {
			n++
		}
	}
	return float64(n) / float64(len(gold))
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
