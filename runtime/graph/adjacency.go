// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package graph

import (
	"fmt"

	"golang.org/x/exp/slices"
)

type adjacencyGraph struct {
	// out[n] stores a list of nodes that n has an outgoing edge to.
	// out[n] == nil means that n is not a node in the graph.
	// out[n] == []Node{} means that node n exists but has no outgoing edges.
	out [][]Node
}

var _ Graph = &adjacencyGraph{}

// NewAdjacencyGraph returns a Graph represented using adjacency lists.
//
// It panics if it specified edge nodes aren't in nodes.
func NewAdjacencyGraph(nodes []Node, edges []Edge) Graph {
	// Find the maximum node value.
	var max Node
	for _, n := range nodes {
		if n > max {
			max = n
		}
	}

	// Build an internal representation.
	out := make([][]Node, max+1)
	for _, n := range nodes {
		out[n] = make([]Node, 0, 1) // provision one outgoing edge by default
	}
	for _, e := range edges {
		if !isNode(e.Src, out) {
			panic(fmt.Sprintf("edge source %d is not a node", e.Src))
		}
		if !isNode(e.Dst, out) {
			panic(fmt.Sprintf("edge destination %d is not a node", e.Src))
		}
		out[e.Src] = append(out[e.Src], e.Dst)
	}

	for _, dsts := range out {
		slices.Sort(dsts)
	}
	return &adjacencyGraph{out: out}
}

var _ Graph = &adjacencyGraph{}

// PerNode implements the Graph interface.
func (g *adjacencyGraph) PerNode(fn func(n Node)) {
	for n, dsts := range g.out {
		if dsts == nil { // not a node
			continue
		}
		fn(Node(n))
	}
}

// PerOutEdge implements the Graph interface.
func (g *adjacencyGraph) PerOutEdge(src Node, fn func(e Edge)) {
	if !isNode(src, g.out) {
		panic(fmt.Sprintf("src %d is not a node", src))
	}
	for _, dst := range g.out[src] {
		fn(Edge{Src: src, Dst: dst})
	}
}

// NodeLimit implements the Graph interface.
func (g *adjacencyGraph) NodeLimit() int {
	return len(g.out)
}

func isNode(n Node, out [][]Node) bool {
	return n >= 0 && int(n) < len(out) && out[n] != nil
}
