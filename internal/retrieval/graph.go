package retrieval

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	"jev/internal/evaluator"
	"jev/internal/graphmodel"
	"jev/internal/judge"
)

// Graph is an undirected, weighted adjacency view of the knowledge graph,
// indexed by position in Nodes. Citations are directed in the source, but a
// question about the cited article may need the citing one and vice versa,
// so traversal goes both ways (as in HippoRAG).
type Graph struct {
	Nodes []*graphmodel.Node
	adj   [][]arc
	index map[*graphmodel.Node]int
}

type arc struct {
	to     int
	weight float64
}

// NewGraph builds the adjacency from nodes and edges. Parallel edges between
// the same pair (A cites B and B cites A) merge into one, keeping the larger
// weight. Self-loops are dropped.
func NewGraph(nodes []*graphmodel.Node, edges []graphmodel.Edge) *Graph {
	g := &Graph{Nodes: nodes, adj: make([][]arc, len(nodes)), index: make(map[*graphmodel.Node]int, len(nodes))}
	for i, n := range nodes {
		g.index[n] = i
	}
	best := map[[2]int]float64{}
	for _, e := range edges {
		u, okU := g.index[e.F]
		v, okV := g.index[e.T]
		if !okU || !okV || u == v {
			continue
		}
		k := pairKey(u, v)
		if w, seen := best[k]; !seen || e.Weight > w {
			best[k] = e.Weight
		}
	}
	keys := make([][2]int, 0, len(best))
	for k := range best {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { // deterministic adjacency order
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	for _, k := range keys {
		w := best[k]
		g.adj[k[0]] = append(g.adj[k[0]], arc{to: k[1], weight: w})
		g.adj[k[1]] = append(g.adj[k[1]], arc{to: k[0], weight: w})
	}
	return g
}

func pairKey(u, v int) [2]int {
	if u > v {
		u, v = v, u
	}
	return [2]int{u, v}
}

// PageRank runs Personalized PageRank: a random walk that, at each step,
// follows a weighted connection with probability damping or jumps back to a
// seed (chosen by reset weight) otherwise. Mass at nodes with no usable
// connection also returns to the seeds. override multiplies the weight of
// specific connections, keyed by pairKey order (min, max); a 0 cuts it.
// Returns one score per node, summing to 1.
func (g *Graph) PageRank(reset map[int]float64, override map[[2]int]float64, damping float64) []float64 {
	n := len(g.Nodes)
	r := make([]float64, n)
	total := 0.0
	for i, w := range reset {
		if w > 0 {
			r[i] = w
			total += w
		}
	}
	if total == 0 {
		return r
	}
	for i := range r {
		r[i] /= total
	}

	// Effective weights, computed once (override lookups are per connection).
	w := make([][]float64, n)
	outSum := make([]float64, n)
	for u := range g.adj {
		w[u] = make([]float64, len(g.adj[u]))
		for j, a := range g.adj[u] {
			x := a.weight
			if m, ok := override[pairKey(u, a.to)]; ok {
				x *= m
			}
			w[u][j] = x
			outSum[u] += x
		}
	}

	p := append([]float64(nil), r...)
	next := make([]float64, n)
	for iter := 0; iter < 100; iter++ {
		dangling := 0.0
		for i := range next {
			next[i] = 0
		}
		for u, pu := range p {
			if pu == 0 {
				continue
			}
			if outSum[u] <= 0 {
				dangling += pu
				continue
			}
			for j, a := range g.adj[u] {
				if w[u][j] > 0 {
					next[a.to] += damping * pu * w[u][j] / outSum[u]
				}
			}
		}
		delta := 0.0
		for i := range next {
			next[i] += (1-damping)*r[i] + damping*dangling*r[i]
			delta += math.Abs(next[i] - p[i])
		}
		p, next = next, p
		if delta < 1e-9 {
			break
		}
	}
	return p
}

// GraphPPR covers options 3, 4 and 5: the same graph and the same
// Personalized PageRank, with or without a judge.
//
// Without a judge (option 3): seeds are the top Seeds nodes by similarity,
// weighted by similarity, and PPR spreads their weight over the graph.
//
// With a judge (options 4 and 5), the judge navigates before PPR runs:
//  1. each seed is judged against the query; its reset weight becomes
//     similarity × tier multiplier (irrelevant seeds drop out);
//  2. from every accepted node (tier high or direct) the judge scores up to
//     Neighbors connections, knowing where it comes from and what is already
//     confirmed; accepted targets are expanded on the next hop, up to Hops;
//  3. each judged connection weighs Weight × multiplier in the PPR (CatRAG);
//     unjudged ones keep their weight.
//
// MaxJudgeCalls caps judge calls per question. The option abstains when the
// judge rejects every seed.
type GraphPPR struct {
	Graph         *Graph
	Judge         judge.Judge // nil for option 3
	Seeds         int
	Neighbors     int
	Hops          int
	MaxJudgeCalls int
	Damping       float64 // probability of following a connection; HippoRAG uses 0.5
	// SeedTemp sharpens seed weights: each seed weighs exp((sim − best)/SeedTemp),
	// so the most similar seed dominates as SeedTemp → 0. 0 uses raw similarity
	// (near-uniform for close similarities, which lets hubs take over).
	SeedTemp    float64
	K           int
	Concurrency int
}

// Retrieve returns the top-K nodes by PPR score.
func (r *GraphPPR) Retrieve(ctx context.Context, q Query) (Result, error) {
	g := r.Graph
	seeds := VectorSearch(q.Embedding, g.Nodes, r.Seeds)
	reset := map[int]float64{}
	override := map[[2]int]float64{}
	res := Result{}

	seeds = r.weightSeeds(seeds)
	if r.Judge == nil {
		for _, s := range seeds {
			reset[g.index[s.Node]] = math.Max(s.Score, 0)
		}
	} else {
		var err error
		res.Judged, err = r.navigate(ctx, q, seeds, reset, override)
		if err != nil {
			return Result{}, err
		}
		if len(reset) == 0 {
			res.Abstained = true
			return res, nil
		}
	}

	scores := g.PageRank(reset, override, r.Damping)
	order := make([]int, 0, len(scores))
	for i, s := range scores {
		if s > 0 {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool { return scores[order[a]] > scores[order[b]] })
	for i := 0; i < len(order) && i < r.K; i++ {
		res.Keys = append(res.Keys, g.Nodes[order[i]].Key)
	}
	if len(res.Keys) == 0 {
		res.Abstained = true
	}
	return res, nil
}

// weightSeeds replaces each seed's similarity by its reset weight.
func (r *GraphPPR) weightSeeds(seeds []Scored) []Scored {
	if r.SeedTemp <= 0 || len(seeds) == 0 {
		return seeds
	}
	out := make([]Scored, len(seeds))
	best := seeds[0].Score
	for i, s := range seeds {
		out[i] = Scored{Node: s.Node, Score: math.Exp((s.Score - best) / r.SeedTemp)}
	}
	return out
}

type judgeTask struct {
	edge graphmodel.Edge
	to   int
}

type judgeResult struct {
	task judgeTask
	d    judge.Decision
	err  error
}

// navigate judges seeds and connections, filling reset weights and edge
// multipliers. It returns the number of judge calls made.
func (r *GraphPPR) navigate(ctx context.Context, q Query, seeds []Scored, reset map[int]float64, override map[[2]int]float64) (int, error) {
	g := r.Graph
	calls := 0
	seen := map[int]bool{} // nodes already judged as a target
	var confirmed []*graphmodel.Node

	// Seeds: edges from the query (no From node).
	var tasks []judgeTask
	for _, s := range seeds {
		if calls+len(tasks) >= r.MaxJudgeCalls {
			break
		}
		i := g.index[s.Node]
		seen[i] = true
		tasks = append(tasks, judgeTask{edge: graphmodel.Edge{T: s.Node, Type: "query_match", Origin: "inferred", Weight: math.Max(s.Score, 0)}, to: i})
	}
	out := r.judgeAll(ctx, q.Text, tasks, nil)
	calls += len(tasks)
	var frontier []int
	for _, jr := range out {
		if jr.err != nil {
			return calls, fmt.Errorf("retrieval: judging seed %q: %w", jr.task.edge.T.Key, jr.err)
		}
		if w := judge.EffectiveWeight(jr.task.edge, jr.d); w > 0 {
			reset[jr.task.to] = w
		}
		if jr.d.Tier >= judge.High {
			confirmed = append(confirmed, jr.task.edge.T)
			frontier = append(frontier, jr.task.to)
		}
	}

	// Hops: connections from accepted nodes, judged with context.
	for hop := 0; hop < r.Hops && len(frontier) > 0; hop++ {
		var next []int
		for _, u := range frontier {
			if calls >= r.MaxJudgeCalls {
				return calls, nil
			}
			cands := r.neighborCandidates(q, u, seen)
			if room := r.MaxJudgeCalls - calls; len(cands) > room {
				cands = cands[:room]
			}
			tasks = tasks[:0]
			for _, a := range cands {
				seen[a.to] = true
				tasks = append(tasks, judgeTask{edge: graphmodel.Edge{F: g.Nodes[u], T: g.Nodes[a.to], Type: "references", Origin: "extracted", Weight: a.weight}, to: a.to})
			}
			snapshot := append([]*graphmodel.Node(nil), confirmed...)
			out := r.judgeAll(ctx, q.Text, tasks, snapshot)
			calls += len(tasks)
			for _, jr := range out {
				if jr.err != nil {
					return calls, fmt.Errorf("retrieval: judging %q → %q: %w", jr.task.edge.F.Key, jr.task.edge.T.Key, jr.err)
				}
				override[pairKey(u, jr.task.to)] = jr.d.Tier.Multiplier()
				if jr.d.Tier >= judge.High {
					confirmed = append(confirmed, jr.task.edge.T)
					next = append(next, jr.task.to)
				}
			}
		}
		frontier = next
	}
	return calls, nil
}

// neighborCandidates returns u's not-yet-judged neighbours, most similar to
// the query first, at most Neighbors of them.
func (r *GraphPPR) neighborCandidates(q Query, u int, seen map[int]bool) []arc {
	g := r.Graph
	var out []arc
	for _, a := range g.adj[u] {
		if !seen[a.to] {
			out = append(out, a)
		}
	}
	sim := func(i int) float64 { return evaluator.CosineSimilarity(q.Embedding, g.Nodes[i].Embedding) }
	sort.SliceStable(out, func(i, j int) bool { return sim(out[i].to) > sim(out[j].to) })
	if len(out) > r.Neighbors {
		out = out[:r.Neighbors]
	}
	return out
}

func (r *GraphPPR) judgeAll(ctx context.Context, query string, tasks []judgeTask, confirmed []*graphmodel.Node) []judgeResult {
	out := make([]judgeResult, len(tasks))
	conc := r.Concurrency
	if conc <= 0 {
		conc = defaultConcurrency
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i, t := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, t judgeTask) {
			defer wg.Done()
			defer func() { <-sem }()
			d, err := r.Judge.ScoreEdge(ctx, t.edge, query, confirmed)
			out[i] = judgeResult{task: t, d: d, err: err}
		}(i, t)
	}
	wg.Wait()
	return out
}
