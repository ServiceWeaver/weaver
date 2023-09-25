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
	"strings"
)

// Node is a small non-negative number that identifies a node in
// a directed graph.  Node numbers should be kept proportional to
// the size of the graph (i.e., mostly densely packed) since different
// algorithms may allocate arrays indexed by the node number.
type Node int

// Edge is a directed graph edge.
type Edge struct {
	Src, Dst Node
}

// Graph is an interface that represents a directed graph.  Most users will
// want to use the `AdjacencyGraph` implementation of `Graph`, but it is
// easy to provide and use a custom `Graph` implementation if necessary.
type Graph interface {
	// PerNode executes the supplied function exactly once per node.
	PerNode(func(n Node))

	// PerOutEdge executes the supplied function exactly once per edge from src.
	PerOutEdge(src Node, fn func(e Edge))

	// NodeLimit returns a number guaranteed to be larger than any
	// Node in the graph.  Many algorithms assume that NodeLimit
	// is not much larger than the number of Nodes in the graph,
	// so Graph implementations should use a relatively dense numeric
	// assignment for nodes.
	NodeLimit() int
}

// PerEdge calls fn(edge) for every edge in g.
func PerEdge(g Graph, fn func(e Edge)) {
	g.PerNode(func(n Node) {
		g.PerOutEdge(n, fn)
	})

}

// OutDegree returns the out-degree for node n in g.
func OutDegree(g Graph, n Node) int {
	var result int
	g.PerOutEdge(n, func(Edge) { result++ })
	return result

}

// DebugString returns a human readable string representation of g.
//
// Sample output for a graph with nodes 1,2,3:
//
//	1:
//	2: 1
//	3: 1 2
func DebugString(g Graph) string {
	var b strings.Builder
	g.PerNode(func(src Node) {
		b.WriteString(fmt.Sprint(src, ":"))
		g.PerOutEdge(src, func(edge Edge) {
			fmt.Fprintf(&b, " %d", edge.Dst)
		})
		b.WriteRune('\n')
	})
	return b.String()
}
