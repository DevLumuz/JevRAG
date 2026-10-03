package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"path/filepath"
	"sort"

	"jev/internal/datasets"
	"jev/internal/embeddings"
	"jev/internal/extract/inferred"
	"jev/internal/extract/legal"
	"jev/internal/graphmodel"
)

// benchQuestion is one evaluation question, independent of the dataset.
type benchQuestion struct {
	ID         string
	Text       string                  // what the judge reads
	EmbedText  string                  // what is embedded for retrieval
	Gold       []string                // keys of the evidence that answers it (canonical form)
	GoldRoles  map[string]string       // optional role per gold key: "final" or "bridge"
	Hops       int                     // reasoning steps / evidence pieces needed
	Answerable bool                    // false: the memory does not contain the answer
	Steps      []datasets.ResolvedStep // optional reasoning decomposition (MuSiQue)
}

// benchmark is a memory (nodes + how they connect) plus questions about it.
// Everything downstream — embeddings, retrieval options, judges, metrics —
// is the same for every benchmark.
type benchmark struct {
	Name      string
	Nodes     []*graphmodel.Node
	Questions []benchQuestion
	// DocText is the exact text embedded for a node.
	DocText func(*graphmodel.Node) string
	// Edges builds the graph; called after node embeddings are set, so
	// similarity-based extractors can use them.
	Edges func(nodes []*graphmodel.Node) []graphmodel.Edge
	// Canon maps a ranked list of node keys to the keys used in Gold.
	Canon func(keys []string) []string
	// Source names the document a node comes from (shown to judges as
	// passage.source): a statute's act/article, a paragraph's title.
	Source func(*graphmodel.Node) string
}

func loadBenchmark(ctx context.Context, hf *datasets.Client, cfg config) (*benchmark, error) {
	switch cfg.dataset {
	case "koblex":
		return loadKoBLEX(ctx, hf, cfg)
	case "musique":
		return loadMuSiQue(ctx, hf, cfg)
	}
	return nil, fmt.Errorf("unknown dataset %q (koblex or musique)", cfg.dataset)
}

// --- KoBLEX: Korean statutes, explicit citations, all questions answerable ---

func loadKoBLEX(ctx context.Context, hf *datasets.Client, cfg config) (*benchmark, error) {
	nodes, edges, err := loadStatutes(ctx, hf, cfg)
	if err != nil {
		return nil, fmt.Errorf("statutes: %w", err)
	}
	rows, err := loadQuestions(ctx, hf, cfg)
	if err != nil {
		return nil, fmt.Errorf("questions: %w", err)
	}
	qs := make([]benchQuestion, len(rows))
	for i, q := range rows {
		qs[i] = benchQuestion{
			ID: q.ID, Text: queryText(q), EmbedText: queryEmbedText(q, cfg.queryVariant),
			Gold: legal.GoldKeys(q), Hops: q.NHops, Answerable: true,
		}
	}
	return &benchmark{
		Name: "koblex", Nodes: nodes, Questions: qs, DocText: docText,
		Edges:  func([]*graphmodel.Node) []graphmodel.Edge { return edges },
		Canon:  legal.CanonicalRanking,
		Source: func(n *graphmodel.Node) string { return n.Key },
	}, nil
}

// --- MuSiQue: Wikipedia paragraphs, no citations, half the questions unanswerable ---

// loadMuSiQue builds one shared memory from the paragraphs of a sample of
// questions: cfg.musiqueN answerable and cfg.musiqueN unanswerable ones, picked
// by ID hash. An unanswerable question is kept only if the paragraph removed
// from it is truly absent from the shared memory (other questions may carry
// it, which would make it answerable).
func loadMuSiQue(ctx context.Context, hf *datasets.Client, cfg config) (*benchmark, error) {
	var raw []map[string]any
	var err error
	if cfg.musiqueSplit == datasets.MuSiQueSplit {
		raw, err = cachedRows(ctx, hf, filepath.Join(cfg.cacheDir, "musique-validation.rows.json"),
			datasets.MuSiQueDataset, datasets.MuSiQueConfig, datasets.MuSiQueSplit)
	} else {
		// A fresh memory from another split (train): never seen while tuning.
		raw, err = sampledRows(ctx, hf, filepath.Join(cfg.cacheDir, fmt.Sprintf("musique-%s-sample%d.rows.json", cfg.musiqueSplit, cfg.musiquePages)),
			datasets.MuSiQueDataset, datasets.MuSiQueConfig, cfg.musiqueSplit, cfg.musiquePages)
	}
	if err != nil {
		return nil, err
	}
	var ans, una []*datasets.MuSiQueRow
	twin := map[string]*datasets.MuSiQueRow{} // answerable row by ID
	for i, r := range raw {
		row, err := datasets.ParseMuSiQueRow(r)
		if err != nil {
			return nil, fmt.Errorf("musique row %d: %w", i, err)
		}
		if row.Answerable {
			ans = append(ans, row)
			twin[row.ID] = row
		} else {
			una = append(una, row)
		}
	}
	byHash := func(rows []*datasets.MuSiQueRow, salt string) {
		sort.SliceStable(rows, func(i, j int) bool { return hash32(salt+rows[i].ID) < hash32(salt+rows[j].ID) })
	}
	byHash(ans, cfg.musiqueSalt+"a")
	byHash(una, cfg.musiqueSalt+"u")

	chosen := ans[:min(cfg.musiqueN, len(ans))]
	taken := map[string]bool{}
	for _, r := range chosen {
		taken[r.ID] = true
	}
	// Candidate unanswerable rows (twins of chosen answerable ones are skipped:
	// they would share the same memory and leak the removed paragraph).
	var unaCands []*datasets.MuSiQueRow
	for _, r := range una {
		if !taken[r.ID] {
			unaCands = append(unaCands, r)
		}
	}

	// Grow the unanswerable set until musiqueN survive the leak check.
	var unaChosen []*datasets.MuSiQueRow
	for size := cfg.musiqueN; ; size += cfg.musiqueN / 2 {
		pool := unaCands[:min(size, len(unaCands))]
		corpus := paragraphSet(append(append([]*datasets.MuSiQueRow{}, chosen...), pool...))
		unaChosen = unaChosen[:0]
		for _, r := range pool {
			if t := twin[r.ID]; t != nil && !allIn(t.Supporting(), corpus) {
				unaChosen = append(unaChosen, r)
			}
		}
		if len(unaChosen) >= cfg.musiqueN || size >= len(unaCands) {
			break
		}
	}
	unaChosen = unaChosen[:min(cfg.musiqueN, len(unaChosen))]

	// The memory: every paragraph of every chosen question, deduplicated.
	rowsUsed := append(append([]*datasets.MuSiQueRow{}, chosen...), unaChosen...)
	index := map[string]*graphmodel.Node{}
	var nodes []*graphmodel.Node
	for _, r := range rowsUsed {
		for _, p := range r.Paragraphs {
			k := datasets.ParagraphKey(p)
			if _, ok := index[k]; ok {
				continue
			}
			n := &graphmodel.Node{
				NumericID: int64(len(nodes)), Key: k, Content: p.Text, Label: "Paragraph",
				Properties: map[string]string{"title": p.Title},
			}
			index[k] = n
			nodes = append(nodes, n)
		}
	}
	// Recheck leaks against the final memory (the last pool may differ).
	var qs []benchQuestion
	corpus := map[string]bool{}
	for k := range index {
		corpus[k] = true
	}
	leaked := 0
	for _, r := range rowsUsed {
		q := benchQuestion{ID: r.ID, Text: r.Question, EmbedText: r.Question, Hops: r.Hops(), Answerable: r.Answerable}
		if r.Answerable {
			for _, p := range r.Supporting() {
				q.Gold = append(q.Gold, datasets.ParagraphKey(p))
			}
			q.GoldRoles = r.SupportRoles()
			q.Steps = r.ResolvedSteps()
		} else if t := twin[r.ID]; t == nil || allIn(t.Supporting(), corpus) {
			leaked++
			continue
		}
		qs = append(qs, q)
	}
	log.Printf("musique memory: %d paragraphs · %d answerable + %d unanswerable questions (%d dropped: answer leaked into memory)",
		len(nodes), len(chosen), len(qs)-len(chosen), leaked)

	return &benchmark{
		Name: musiqueName(cfg), Nodes: nodes, Questions: qs,
		DocText: func(n *graphmodel.Node) string {
			return truncateRunes(n.Properties["title"]+" — "+n.Content, maxEmbedChars)
		},
		Edges: func(nodes []*graphmodel.Node) []graphmodel.Edge {
			var edges []graphmodel.Edge
			if cfg.inferEdges == "mentions" || cfg.inferEdges == "both" {
				edges = append(edges, inferred.MentionEdges(nodes)...)
			}
			if cfg.inferEdges == "similar" || cfg.inferEdges == "both" {
				edges = append(edges, similarityEdges(nodes, 5)...)
			}
			return edges
		},
		Canon:  func(keys []string) []string { return keys },
		Source: func(n *graphmodel.Node) string { return n.Properties["title"] },
	}, nil
}

// similarityEdges runs k-NN on 256-dim reduced copies of the embeddings
// (O(n²); full vectors would be needlessly slow) and maps edges back.
func similarityEdges(nodes []*graphmodel.Node, k int) []graphmodel.Edge {
	shadow := make([]*graphmodel.Node, len(nodes))
	back := make(map[*graphmodel.Node]*graphmodel.Node, len(nodes))
	for i, n := range nodes {
		shadow[i] = &graphmodel.Node{Key: n.Key, Embedding: embeddings.Reduce(n.Embedding, 256)}
		back[shadow[i]] = n
	}
	edges := inferred.SimilarityEdges(shadow, k, 0)
	for i := range edges {
		edges[i].F, edges[i].T = back[edges[i].F], back[edges[i].T]
	}
	return edges
}

func paragraphSet(rows []*datasets.MuSiQueRow) map[string]bool {
	set := map[string]bool{}
	for _, r := range rows {
		for _, p := range r.Paragraphs {
			set[datasets.ParagraphKey(p)] = true
		}
	}
	return set
}

func allIn(ps []datasets.MuSiQueParagraph, set map[string]bool) bool {
	for _, p := range ps {
		if !set[datasets.ParagraphKey(p)] {
			return false
		}
	}
	return len(ps) > 0
}

func hash32(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// splitQuestions assigns questions to dev/test by ID hash (same rule as KoBLEX).
func splitQuestions(qs []benchQuestion, devFraction float64) (dev, test []benchQuestion) {
	for _, q := range qs {
		if datasets.InDev(q.ID, devFraction) {
			dev = append(dev, q)
		} else {
			test = append(test, q)
		}
	}
	return dev, test
}

// musiqueName tags results of a fresh memory with its split.
func musiqueName(cfg config) string {
	if cfg.musiqueSplit == datasets.MuSiQueSplit {
		return "musique"
	}
	return "musique-" + cfg.musiqueSplit + cfg.musiqueSalt
}
