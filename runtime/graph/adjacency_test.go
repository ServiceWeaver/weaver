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

package graph_test

import (
	"sort"
	"testing"

	"github.com/ServiceWeaver/weaver/runtime/graph"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/exp/slices"
)

func TestAdjacencyGraph(t *testing.T) {
	for _, tc := range []struct {
		name  string
		nodes []graph.Node
		edges []graph.Edge
	}{
		{name: "empty"},
		{
			name:  "nodes_without_edges",
			nodes: []graph.Node{0, 1, 2, 3},
		},
		{
			name:  "nodes_with_edges",
			nodes: []graph.Node{0, 1, 2, 3},
			edges: []graph.Edge{{0, 0}, {0, 1}, {1, 2}, {2, 3}, {3, 0}},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := graph.NewAdjacencyGraph(tc.nodes, tc.edges)

			// Collect nodes from the graph and compare.
			var nodes []graph.Node
			g.PerNode(func(n graph.Node) {
				nodes = append(nodes, n)
			})
			slices.Sort(nodes)
			if diff := cmp.Diff(tc.nodes, nodes); diff != "" {
				t.Fatalf("unexpected nodes: (-want +got): %s", diff)
			}

			// Collect edges from the graph and compare.
			var edges []graph.Edge
			graph.PerEdge(g, func(e graph.Edge) {
				edges = append(edges, e)
			})
			sort.Slice(edges, func(i, j int) bool {
				x, y := edges[i], edges[j]
				if x.Src == y.Src {
					return x.Dst < y.Dst
				}
				return x.Src < y.Src
			})
			if diff := cmp.Diff(tc.edges, edges); diff != "" {
				t.Fatalf("unexpected edges: (-want +got): %s", diff)
			}

		})
	}
}
