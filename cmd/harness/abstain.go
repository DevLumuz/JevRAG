package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"jev/internal/graphmodel"
	"jev/internal/judge"
	"jev/internal/notebook"
	"jev/internal/probe"
	"jev/internal/retrieval"
)

// abstainRow is one question's abstention signals (higher = "answer").
type abstainRow struct {
	ID         string  `json:"id"`
	Answerable bool    `json:"answerable"`
	Hops       int     `json:"hops"`
	OldGate    float64 `json:"old_gate"`      // earlier gate: JEV on the top 5 vector passages
	Stated     float64 `json:"answer_stated"` // notebook: final answer stated
	NoMissing  float64 `json:"no_missing"`    // notebook: 1 − P(missing link)
	Coverage   float64 `json:"coverage"`      // mean of the two notebook signals
	MaxAnswer  float64 `json:"max_answer"`    // loop: highest P(answers_query) of any passage
	LoopGate   float64 `json:"loop_gate"`     // earlier gate question on the loop's top 5 passages
	CovPass    float64 `json:"cov_passages"`  // coverage questions on notebook + loop's top 5 passages
	Facts      int     `json:"facts"`
}

var abstainSignals = []struct {
	name    string
	get     func(abstainRow) float64
	primary bool
}{
	{"earlier gate (JEV on top-5 vector passages)", func(r abstainRow) float64 { return r.OldGate }, false},
	{"notebook: answer_stated", func(r abstainRow) float64 { return r.Stated }, false},
	{"notebook: 1 − missing_link", func(r abstainRow) float64 { return r.NoMissing }, false},
	{"notebook: coverage (mean of both)", func(r abstainRow) float64 { return r.Coverage }, false},
	{"mean(earlier gate, notebook coverage) — primary (frozen in plan §26)", func(r abstainRow) float64 { return (r.OldGate + r.Coverage) / 2 }, true},
	{"loop: max answers_query over passages", func(r abstainRow) float64 { return r.MaxAnswer }, false},
	{"earlier gate question on the loop's top 5", func(r abstainRow) float64 { return r.LoopGate }, false},
	{"coverage on notebook + loop's top 5 passages", func(r abstainRow) float64 { return r.CovPass }, false},
	{"mean(earlier gate on loop top 5, coverage + passages)", func(r abstainRow) float64 { return (r.LoopGate + r.CovPass) / 2 }, false},
}

// runAbstain is Phase 3: after the notebook loop, decide "not enough
// evidence" from the notebook alone, against the earlier gate.
func runAbstain(ctx context.Context, cfg config, bench *benchmark, qs []benchQuestion, qEmb map[string][]float64, querySpace *embedSpace) error {
	ex, loop, err := newExplorer(cfg, bench, querySpace)
	if err != nil {
		return err
	}
	var rows []abstainRow
	var paid int64
	for i, q := range qs {
		tr, err := ex.Explore(ctx, q.Text, qEmb[q.ID], loop)
		if err != nil {
			return fmt.Errorf("%s: %w", q.ID, err)
		}
		top := retrieval.VectorSearch(qEmb[q.ID], bench.Nodes, 5)
		ev := judge.EvidenceState{Query: q.Text}
		for _, s := range top {
			ev.Evidence = append(ev.Evidence, judge.EvidenceItem{ID: s.Node.Key, Text: s.Node.Content})
		}
		byKey := map[string]*graphmodel.Node{}
		for _, n := range bench.Nodes {
			byKey[n.Key] = n
		}
		lev := judge.EvidenceState{Query: q.Text}
		var passages []notebook.Passage
		for _, k := range tr.Ranked[:min(5, len(tr.Ranked))] {
			n := byKey[k]
			lev.Evidence = append(lev.Evidence, judge.EvidenceItem{ID: n.Key, Text: n.Content})
			passages = append(passages, notebook.Passage{Title: bench.Source(n), Text: n.Content})
		}
		items := []probe.Item{
			{State: ev, Questions: judge.SufficiencyQuestions()},
			{State: notebook.CoverageState{Query: q.Text, KnownFacts: notebook.View(tr.Notebook)}, Questions: notebook.CoverageQuestions},
			{State: lev, Questions: judge.SufficiencyQuestions()},
			{State: notebook.CoverageWithPassages{Query: q.Text, KnownFacts: notebook.View(tr.Notebook), Passages: passages}, Questions: notebook.CoverageQuestions},
		}
		resps, u, err := probe.RunItems(ctx, ex.JEV, ex.Model, items, ex.Cache, 2, 0)
		if err != nil {
			return fmt.Errorf("%s: %w", q.ID, err)
		}
		paid += tr.Tokens + u.InputTokens
		r := abstainRow{ID: q.ID, Answerable: q.Answerable, Hops: q.Hops, OldGate: judge.SufficiencyAnswer(resps[0]),
			Stated: resps[1].Answers["answer_stated"].Noul, NoMissing: 1 - resps[1].Answers["missing_link"].Noul,
			MaxAnswer: tr.MaxAnswer, Facts: len(tr.Notebook)}
		r.Coverage = (r.Stated + r.NoMissing) / 2
		r.LoopGate = judge.SufficiencyAnswer(resps[2])
		r.CovPass = (resps[3].Answers["answer_stated"].Noul + 1 - resps[3].Answers["missing_link"].Noul) / 2
		rows = append(rows, r)
		if (i+1)%20 == 0 {
			log.Printf("  %d/%d · paid JEV tokens so far %d (~US$%.3f)", i+1, len(qs), paid, float64(paid)/1e6*jevPricePerMTok)
		}
	}

	var b strings.Builder
	nA, nU := 0, 0
	for _, r := range rows {
		if r.Answerable {
			nA++
		} else {
			nU++
		}
	}
	fmt.Fprintf(&b, "# Phase 3 — abstention from the notebook (%s, %s)\n\n", bench.Name, cfg.mode)
	fmt.Fprintf(&b, "%d questions: %d answerable, %d unanswerable (the memory lacks a needed passage). Paid JEV this run: ~US$%.3f.\n\n", len(rows), nA, nU, float64(paid)/1e6*jevPricePerMTok)
	b.WriteString("AUROC = probability that a random answerable question scores above a random unanswerable one (95% CI resampling questions). Balanced accuracy = mean of the hit rate on each class.\n\n")
	b.WriteString("| signal | AUROC | best balanced acc. (threshold) | balanced acc. at --abstain-threshold |\n|---|---|---|---|\n")
	for _, sg := range abstainSignals {
		auc, lo, hi := aurocCI(rows, sg.get)
		bestT, bestAcc := bestThreshold(rows, sg.get)
		frozen := "—"
		if sg.primary {
			frozen = fmt.Sprintf("%.3f", balancedAcc(rows, sg.get, cfg.abstainThreshold))
		}
		fmt.Fprintf(&b, "| %s | %.3f [%.3f–%.3f] | %.3f (%.2f) | %s |\n", sg.name, auc, lo, hi, bestAcc, bestT, frozen)
	}
	fmt.Fprintf(&b, "\nFrozen threshold (--abstain-threshold) = %.2f, applied to the primary signal; it is chosen on dev only.\n", cfg.abstainThreshold)
	fmt.Print(b.String())

	stamp := time.Now().Format("20060102-150405")
	out := filepath.Join(cfg.resultsDir, fmt.Sprintf("%s-%s-abstain-%s.md", stamp, bench.Name, cfg.mode))
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		return err
	}
	if err := writeJSON(strings.TrimSuffix(out, ".md")+"-rows.json", rows); err != nil {
		return err
	}
	log.Printf("report: %s", out)
	return nil
}

func aurocCI(rows []abstainRow, get func(abstainRow) float64) (auc, lo, hi float64) {
	split := func(rs []abstainRow) (pos, neg []float64) {
		for _, r := range rs {
			if r.Answerable {
				pos = append(pos, get(r))
			} else {
				neg = append(neg, get(r))
			}
		}
		return
	}
	pos, neg := split(rows)
	auc = probe.AUC(pos, neg)
	rng := uint64(12345)
	next := func() uint64 { rng ^= rng << 13; rng ^= rng >> 7; rng ^= rng << 17; return rng }
	var bs []float64
	for k := 0; k < 2000; k++ {
		rs := make([]abstainRow, len(rows))
		for i := range rs {
			rs[i] = rows[next()%uint64(len(rows))]
		}
		p, n := split(rs)
		if v := probe.AUC(p, n); !math.IsNaN(v) {
			bs = append(bs, v)
		}
	}
	sort.Float64s(bs)
	return auc, bs[len(bs)*25/1000], bs[len(bs)*975/1000-1]
}

// balancedAcc: answer when the signal is ≥ t, abstain otherwise.
func balancedAcc(rows []abstainRow, get func(abstainRow) float64, t float64) float64 {
	var tp, fn, tn, fp float64
	for _, r := range rows {
		ans := get(r) >= t
		switch {
		case r.Answerable && ans:
			tp++
		case r.Answerable:
			fn++
		case ans:
			fp++
		default:
			tn++
		}
	}
	if tp+fn == 0 || tn+fp == 0 {
		return math.NaN()
	}
	return (tp/(tp+fn) + tn/(tn+fp)) / 2
}

func bestThreshold(rows []abstainRow, get func(abstainRow) float64) (float64, float64) {
	bestT, bestAcc := 0.0, -1.0
	for t := 0.01; t < 1; t += 0.01 {
		if a := balancedAcc(rows, get, t); a > bestAcc {
			bestT, bestAcc = t, a
		}
	}
	return bestT, bestAcc
}
