// Command probe measures JEV question sets on the labeled dev pairs built by
// `harness --build-probe-bank`, without running full retrieval.
//
//	go run ./cmd/probe --sets questionsets/passage-v1.json,questionsets/passage-v2.json
//
// For every question it reports how well it separates gold passages from
// hard negatives (AUC), per dataset and per gold role (final answer vs.
// intermediate "bridge"). It also fits a combination of all the set's
// answers (logistic regression) on one dataset and scores it on the other,
// which shows whether the set is domain-agnostic. Responses are cached by
// content hash in data/probe/cache.jsonl, so re-running costs nothing.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"jev/internal/jev"
	"jev/internal/judge"
	"jev/internal/probe"
)

const jevPricePerMTok = 0.042

func main() {
	log.SetFlags(0)
	sets := flag.String("sets", "", "comma-separated question set files")
	banks := flag.String("banks", "data/probe/bank-dev-koblex.json,data/probe/bank-dev-musique.json", "comma-separated bank files")
	cachePath := flag.String("cache", "data/probe/cache.jsonl", "response cache")
	outDir := flag.String("out", "results/probe", "where reports are written")
	model := flag.String("model", "jev-latest", "JEV model")
	conc := flag.Int("concurrency", 8, "parallel requests")
	maxTok := flag.Int64("max-jev-tokens", 2_000_000, "cap on paid input tokens per set (~US$0.042 per million)")
	flag.Parse()
	if *sets == "" {
		log.Fatal("probe: --sets is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var pairs []probe.Pair
	for _, b := range strings.Split(*banks, ",") {
		ps, err := probe.LoadPairs(b)
		if err != nil {
			log.Fatalf("probe: %v", err)
		}
		pairs = append(pairs, ps...)
	}
	client, err := jev.NewHTTPClient(jev.Options{Model: *model})
	if err != nil {
		log.Fatalf("probe: %v", err)
	}
	cache, err := probe.OpenCache(*cachePath)
	if err != nil {
		log.Fatalf("probe: %v", err)
	}

	for _, path := range strings.Split(*sets, ",") {
		set, err := judge.LoadQuestionSet(path)
		if err != nil {
			log.Fatalf("probe: %v", err)
		}
		start := time.Now()
		feats, usage, err := probe.Run(ctx, client, *model, set, pairs, cache, *conc, *maxTok)
		if err != nil {
			log.Fatalf("probe: %s: %v", set.Name, err)
		}
		report := render(set, pairs, feats, usage, time.Since(start))
		fmt.Print(report)
		out := filepath.Join(*outDir, set.Name+".md")
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(out, []byte(report), 0o644); err != nil {
			log.Fatal(err)
		}
		log.Printf("report: %s", out)
	}
}

// group selects pairs; pos/neg split the selection into classes.
type group struct {
	name     string
	pos, neg func(probe.Pair) bool
}

func isDS(ds string) func(probe.Pair) bool { return func(p probe.Pair) bool { return p.Dataset == ds } }

func and(fs ...func(probe.Pair) bool) func(probe.Pair) bool {
	return func(p probe.Pair) bool {
		for _, f := range fs {
			if !f(p) {
				return false
			}
		}
		return true
	}
}

func role(r ...string) func(probe.Pair) bool {
	return func(p probe.Pair) bool {
		for _, x := range r {
			if p.Role == x {
				return true
			}
		}
		return false
	}
}

var (
	gold = func(p probe.Pair) bool { return p.Gold() }
	hard = role(probe.RoleHardNegative)
)

var groups = []group{
	{"all: gold vs hard-neg", gold, hard},
	{"koblex: gold vs hard-neg", and(isDS("koblex"), gold), and(isDS("koblex"), hard)},
	{"musique: gold vs hard-neg", and(isDS("musique"), gold), and(isDS("musique"), hard)},
	{"musique: FINAL vs hard-neg", and(isDS("musique"), role(probe.RoleFinal)), and(isDS("musique"), hard)},
	{"musique: BRIDGE vs hard-neg", and(isDS("musique"), role(probe.RoleBridge)), and(isDS("musique"), hard)},
	{"musique: gold vs unanswerable-top", and(isDS("musique"), gold), and(isDS("musique"), role(probe.RoleUnanswerable))},
}

func auc(pairs []probe.Pair, score func(i int) float64, g group) float64 {
	var pos, neg []float64
	for i, p := range pairs {
		switch {
		case g.pos(p):
			pos = append(pos, score(i))
		case g.neg(p):
			neg = append(neg, score(i))
		}
	}
	return probe.AUC(pos, neg)
}

func fmtAUC(v float64) string {
	if math.IsNaN(v) {
		return "  —  "
	}
	return fmt.Sprintf("%.3f", v)
}

func render(set *judge.QuestionSet, pairs []probe.Pair, feats []map[string]float64, u probe.Usage, took time.Duration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Probe report: %s\n\n%s\n\n", set.Name, set.Description)
	fmt.Fprintf(&b, "%d pairs · %d paid calls · %d cache hits · %d input tokens (~US$%.3f) · %v\n\n",
		len(pairs), u.Calls, u.CacheHits, u.InputTokens, float64(u.InputTokens)/1e6*jevPricePerMTok, took.Round(time.Second))

	names := featureNames(feats)
	fmt.Fprintf(&b, "AUC (0.5 = no separation, 1 = perfect). Reference row: embedding rank alone.\n\n")
	fmt.Fprintf(&b, "| feature |")
	for _, g := range groups {
		fmt.Fprintf(&b, " %s |", g.name)
	}
	fmt.Fprintf(&b, "\n|---|%s\n", strings.Repeat("---|", len(groups)))
	row := func(name string, score func(i int) float64) {
		fmt.Fprintf(&b, "| %s |", name)
		for _, g := range groups {
			fmt.Fprintf(&b, " %s |", fmtAUC(auc(pairs, score, g)))
		}
		b.WriteString("\n")
	}
	row("_embedding rank_", func(i int) float64 { return rankScore(pairs[i]) })
	for _, n := range names {
		n := n
		row("`"+n+"`", func(i int) float64 { return feats[i][n] })
	}

	// Composite: learn on one dataset, score the other (agnosticism check),
	// with and without the embedding rank as an extra feature.
	b.WriteString("\n## Combined score (logistic regression, trained on one dataset, scored on the other)\n\n")
	b.WriteString("| combination | train koblex → musique gold vs hard-neg | train musique → koblex gold vs hard-neg | musique BRIDGE vs hard-neg (train koblex) |\n|---|---|---|---|\n")
	for _, withRank := range []bool{false, true} {
		cols := names
		label := "JEV answers"
		if withRank {
			label = "JEV answers + embedding rank"
		}
		x := func(i int) []float64 {
			v := make([]float64, 0, len(cols)+1)
			for _, n := range cols {
				v = append(v, feats[i][n])
			}
			if withRank {
				v = append(v, rankScore(pairs[i]))
			}
			return v
		}
		fit := func(ds string) *probe.Logistic {
			var xs [][]float64
			var ys []float64
			for i, p := range pairs {
				if p.Dataset != ds || !(p.Gold() || p.Role == probe.RoleHardNegative) {
					continue
				}
				xs = append(xs, x(i))
				y := 0.0
				if p.Gold() {
					y = 1
				}
				ys = append(ys, y)
			}
			fnames := append([]string{}, cols...)
			if withRank {
				fnames = append(fnames, "rank")
			}
			return probe.FitLogistic(fnames, xs, ys)
		}
		mk, mm := fit("koblex"), fit("musique")
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", label,
			fmtAUC(auc(pairs, func(i int) float64 { return mk.Score(x(i)) }, groups[2])),
			fmtAUC(auc(pairs, func(i int) float64 { return mm.Score(x(i)) }, groups[1])),
			fmtAUC(auc(pairs, func(i int) float64 { return mk.Score(x(i)) }, groups[4])))
	}

	b.WriteString("\n## Mean answer by passage role\n\n| feature | final | bridge | gold (statutes) | hard-neg | unanswerable-top |\n|---|---|---|---|---|---|\n")
	for _, n := range names {
		fmt.Fprintf(&b, "| `%s` |", n)
		for _, r := range []string{probe.RoleFinal, probe.RoleBridge, probe.RoleGold, probe.RoleHardNegative, probe.RoleUnanswerable} {
			sum, c := 0.0, 0
			for i, p := range pairs {
				if p.Role == r {
					sum += feats[i][n]
					c++
				}
			}
			if c == 0 {
				b.WriteString(" — |")
			} else {
				fmt.Fprintf(&b, " %.2f |", sum/float64(c))
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// rankScore turns the embedding rank into a higher-is-better score; passages
// outside the shortlist get the lowest score.
func rankScore(p probe.Pair) float64 {
	if p.Rank <= 0 {
		return -100
	}
	return -float64(p.Rank)
}

func featureNames(feats []map[string]float64) []string {
	seen := map[string]bool{}
	for _, f := range feats {
		for k := range f {
			seen[k] = true
		}
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
