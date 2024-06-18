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
	"testing"

	"github.com/ServiceWeaver/weaver/runtime/graph"
)

func TestGraphHasCycle(t *testing.T) {
	testCases := []struct {
		name     string
		graph    graph.Graph
		expected bool
	}{
		{
			name: "HasCycle",
			graph: graph.NewAdjacencyGraph(
				// [0 1 2 0 1 2 0 1 2]
				// [{1 2} {2 1}]
				[]graph.Node{1, 2, 3, 4},
				[]graph.Edge{
					{1, 2},
					{2, 1},
					{2, 3},
					{3, 2},
					{3, 4},
				},
			),
			expected: true,
		},
		{
			name: "HasNoCycle",
			graph: graph.NewAdjacencyGraph(
				[]graph.Node{1, 2, 3, 4},
				[]graph.Edge{
					{1, 2},
					{2, 3},
					{3, 4},
				},
			),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hasCycle := graph.HasCycle(tc.graph)
			if hasCycle != tc.expected {
				t.Errorf("%s = %t, want %t", tc.name, hasCycle, tc.expected)
			}
		})
	}
}
