// Package graphmodel defines the generic labeled property graph used by the
// evaluation harness. Node and Edge work for any domain — legal articles,
// movies, documentation, etc. The domain-specific extraction layer produces
// these types; everything downstream (traversal, judge, evaluator) consumes them.
package graphmodel

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/simple"
)

// Node is a generic graph node: any chunk or document, from any domain.
// Implements graph.Node (gonum requires an ID() int64 method).
type Node struct {
	NumericID   int64             `json:"numeric_id"`
	Key         string            `json:"key"`
	Content     string            `json:"content"`
	Label       string            `json:"label"`
	Properties  map[string]string `json:"properties,omitempty"`
	Embedding   []float64         `json:"embedding,omitempty"`
	LastUpdated time.Time         `json:"last_updated"`
}

// ID satisfies gonum's graph.Node interface.
func (n *Node) ID() int64 { return n.NumericID }

// Edge is a directed edge between two Nodes.
type Edge struct {
	F          *Node             `json:"-"`
	T          *Node             `json:"-"`
	Type       string            `json:"type"`
	Origin     string            `json:"origin"`
	Weight     float64           `json:"weight"`
	Properties map[string]string `json:"properties,omitempty"`
}

// From satisfies gonum's graph.Edge interface.
func (e Edge) From() graph.Node { return e.F }

// To satisfies gonum's graph.Edge interface.
func (e Edge) To() graph.Node { return e.T }

// ReversedEdge satisfies gonum's graph.Edge interface.
func (e Edge) ReversedEdge() graph.Edge {
	return Edge{
		F: e.T, T: e.F,
		Type: e.Type, Origin: e.Origin, Weight: e.Weight,
		Properties: e.Properties,
	}
}

// Weight returns the static edge weight (for gonum's graph.Weighted interface).
func (e Edge) WeightVal() float64 { return e.Weight }

// BuildGraph builds a gonum directed graph from pre-extracted nodes and edges.
func BuildGraph(nodes []*Node, edges []Edge) *simple.DirectedGraph {
	g := simple.NewDirectedGraph()
	for _, n := range nodes {
		g.AddNode(n)
	}
	for _, e := range edges {
		g.SetEdge(e)
	}
	return g
}

// --- JSON Persistence ---

// edgeJSON is the serialization format for edges — stores keys instead of
// pointers so the file is self-contained.
type edgeJSON struct {
	FromKey    string            `json:"from_key"`
	ToKey      string            `json:"to_key"`
	Type       string            `json:"type"`
	Origin     string            `json:"origin"`
	Weight     float64           `json:"weight"`
	Properties map[string]string `json:"properties,omitempty"`
}

type graphJSON struct {
	Nodes []*Node    `json:"nodes"`
	Edges []edgeJSON `json:"edges"`
}

// SaveGraph persists nodes and edges to a JSON file.
func SaveGraph(path string, nodes []*Node, edges []Edge) error {
	gj := graphJSON{Nodes: nodes}
	for _, e := range edges {
		gj.Edges = append(gj.Edges, edgeJSON{
			FromKey:    e.F.Key,
			ToKey:      e.T.Key,
			Type:       e.Type,
			Origin:     e.Origin,
			Weight:     e.Weight,
			Properties: e.Properties,
		})
	}

	data, err := json.MarshalIndent(gj, "", "  ")
	if err != nil {
		return fmt.Errorf("graphmodel: marshaling: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("graphmodel: writing %s: %w", path, err)
	}
	return nil
}

// LoadGraph reads nodes and edges from a JSON file previously saved by SaveGraph.
// It rebuilds the pointer links between edges and nodes.
func LoadGraph(path string) ([]*Node, []Edge, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("graphmodel: reading %s: %w", path, err)
	}

	var gj graphJSON
	if err := json.Unmarshal(data, &gj); err != nil {
		return nil, nil, fmt.Errorf("graphmodel: parsing %s: %w", path, err)
	}

	// Build key→node index for resolving edge pointers.
	byKey := make(map[string]*Node, len(gj.Nodes))
	for _, n := range gj.Nodes {
		byKey[n.Key] = n
	}

	edges := make([]Edge, 0, len(gj.Edges))
	for _, ej := range gj.Edges {
		from, ok := byKey[ej.FromKey]
		if !ok {
			return nil, nil, fmt.Errorf("graphmodel: edge references unknown from_key %q", ej.FromKey)
		}
		to, ok := byKey[ej.ToKey]
		if !ok {
			return nil, nil, fmt.Errorf("graphmodel: edge references unknown to_key %q", ej.ToKey)
		}
		edges = append(edges, Edge{
			F: from, T: to,
			Type: ej.Type, Origin: ej.Origin, Weight: ej.Weight,
			Properties: ej.Properties,
		})
	}

	return gj.Nodes, edges, nil
}
