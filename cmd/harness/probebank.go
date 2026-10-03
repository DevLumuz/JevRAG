package main

import (
	"fmt"
	"log"
	"path/filepath"

	"jev/internal/graphmodel"
	"jev/internal/probe"
	"jev/internal/retrieval"
)

const (
	bankPool          = 30 // embedding shortlist size (same as option 2 candidates)
	bankHardNegatives = 3  // non-gold passages per question, taken from the top of the shortlist
)

// buildProbeBank writes labeled (query, passage) pairs from the DEV split only:
// every gold passage (with its role when known), the top non-gold passages of
// the embedding shortlist as hard negatives, and the top passages of
// unanswerable questions. Test questions are never touched.
func buildProbeBank(bench *benchmark, dev []benchQuestion, qEmb map[string][]float64, outDir string) (string, error) {
	nodes := bench.Nodes
	byCanon := map[string]*graphmodel.Node{}
	for _, n := range nodes {
		k := bench.Canon([]string{n.Key})[0]
		if _, ok := byCanon[k]; !ok {
			byCanon[k] = n
		}
	}

	var pairs []probe.Pair
	counts := map[string]int{}
	add := func(q benchQuestion, n *graphmodel.Node, role string, rank int) {
		pairs = append(pairs, probe.Pair{
			Dataset: bench.Name, QueryID: q.ID, Query: q.Text, Hops: q.Hops,
			PassageID: n.Key, Source: bench.Source(n), Text: n.Content, Role: role, Rank: rank,
		})
		counts[role]++
	}

	for _, q := range dev {
		pool := retrieval.VectorSearch(qEmb[q.ID], nodes, bankPool)
		keys := make([]string, len(pool))
		for i, s := range pool {
			keys[i] = s.Node.Key
		}
		ranked := bench.Canon(keys)
		rank := map[string]int{}
		for i, k := range ranked {
			rank[k] = i + 1
		}

		if !q.Answerable {
			for i := 0; i < len(ranked) && i < bankHardNegatives; i++ {
				add(q, byCanon[ranked[i]], probe.RoleUnanswerable, i+1)
			}
			continue
		}
		gold := map[string]bool{}
		for _, g := range q.Gold {
			gold[g] = true
			n, ok := byCanon[g]
			if !ok {
				continue // gold not in the memory (known corpus gaps)
			}
			role := probe.RoleGold
			if r, ok := q.GoldRoles[g]; ok {
				role = r
			}
			add(q, n, role, rank[g])
		}
		taken := 0
		for i, k := range ranked {
			if taken == bankHardNegatives {
				break
			}
			if !gold[k] {
				add(q, byCanon[k], probe.RoleHardNegative, i+1)
				taken++
			}
		}
	}

	path := filepath.Join(outDir, fmt.Sprintf("bank-dev-%s.json", bench.Name))
	if err := writeJSON(path, pairs); err != nil {
		return "", err
	}
	log.Printf("probe bank %s: %d pairs %v", path, len(pairs), counts)
	return path, nil
}
