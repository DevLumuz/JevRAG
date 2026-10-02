package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"jev/internal/datasets"
	"jev/internal/extract/legal"
)

// runCheck downloads (or reads cached) KoBLEX data, builds the citation graph
// without embeddings, and reports whether the gold provisions of every
// question can be found in the corpus. It needs no API keys: run it before
// paying for any embedding or judge call.
func runCheck(ctx context.Context, hf *datasets.Client, cfg config) error {
	nodes, edges, err := loadStatutes(ctx, hf, cfg)
	if err != nil {
		return fmt.Errorf("statutes: %w", err)
	}
	qs, err := loadQuestions(ctx, hf, cfg)
	if err != nil {
		return fmt.Errorf("questions: %w", err)
	}

	// Corpus: canonical keys, duplicates, machine translations, sizes.
	byCanon := make(map[string]int, len(nodes))
	acts := map[string]bool{}
	machine, empty, totalChars := 0, 0, 0
	for _, n := range nodes {
		byCanon[legal.CanonicalKey(n.Key)]++
		acts[n.Properties["act"]] = true
		if strings.Contains(n.Content, "MACHINE_TRANSLATED") {
			machine++
		}
		if n.Properties["article_number"] == "" {
			empty++
		}
		totalChars += len(n.Content)
	}
	dupes := 0
	for _, c := range byCanon {
		if c > 1 {
			dupes++
		}
	}
	withOut := map[string]bool{}
	for _, e := range edges {
		withOut[e.F.Key] = true
	}

	fmt.Printf("\n== corpus (%s) ==\n", koblexStatutes)
	fmt.Printf("statutes            %d (%d acts)\n", len(nodes), len(acts))
	fmt.Printf("avg content         %d chars\n", totalChars/max(len(nodes), 1))
	fmt.Printf("no article number   %d\n", empty)
	fmt.Printf("duplicate act#art   %d keys\n", dupes)
	fmt.Printf("MACHINE_TRANSLATED  %d\n", machine)
	fmt.Printf("citation edges      %d (%d nodes cite something)\n", len(edges), len(withOut))

	// Questions: gold coverage and split sizes.
	hops := map[int]int{}
	goldTotal, goldFound, fullyCovered := 0, 0, 0
	var missing []string
	for _, q := range qs {
		hops[q.NHops]++
		gold := legal.GoldKeys(q)
		found := 0
		for _, g := range gold {
			if byCanon[g] > 0 {
				found++
			} else if len(missing) < 10 {
				missing = append(missing, fmt.Sprintf("%s → %s", q.ID, g))
			}
		}
		goldTotal += len(gold)
		goldFound += found
		if found == len(gold) {
			fullyCovered++
		}
	}
	dev, test := datasets.SplitDevTest(qs, cfg.devFraction)

	fmt.Printf("\n== questions (%s) ==\n", koblexQA)
	fmt.Printf("questions           %d (dev %d · test %d)\n", len(qs), len(dev), len(test))
	hk := make([]int, 0, len(hops))
	for h := range hops {
		hk = append(hk, h)
	}
	sort.Ints(hk)
	for _, h := range hk {
		fmt.Printf("  %d-hop             %d\n", h, hops[h])
	}
	fmt.Printf("gold provisions     %d, found in corpus %d (%.1f%%)\n", goldTotal, goldFound, 100*float64(goldFound)/float64(max(goldTotal, 1)))
	fmt.Printf("fully covered qs    %d/%d\n", fullyCovered, len(qs))
	for _, m := range missing {
		fmt.Printf("  missing: %s\n", m)
	}
	return nil
}
