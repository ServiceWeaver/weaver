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

import "slices"

// DFSAll performs a depth first search of all nodes in g.
// If enter is non-nil, it is called on entry to a node.
// If exit is non-nil, it is called on exit from a node.
func DFSAll(g Graph, enter, exit func(Node)) {
	var all []Node
	g.PerNode(func(n Node) {
		all = append(all, n)
	})
	dfs(g, all, enter, exit)
}

// PostOrder returns nodes in g in post-order.
func PostOrder(g Graph) []Node {
	var result []Node
	DFSAll(g, nil, func(n Node) {
		result = append(result, n)
	})
	return result
}

// ReversePostOrder returns nodes in g in reverse-post-order.
func ReversePostOrder(g Graph) []Node {
	result := PostOrder(g)
	slices.Reverse(result)
	return result
}

func dfs(g Graph, roots []Node, enter, exit func(Node)) {
	// Stack holds nodes to traverse.  If we need to call exit, we
	// leave a negative marker at the appropriate place in the stack.
	var stack []Node
	visited := make([]bool, g.NodeLimit())
	for _, r := range roots {
		stack = append(stack, r)
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if n < 0 {
				exit(-n - 1)
				continue
			}
			if visited[n] {
				continue
			}
			visited[n] = true
			if exit != nil {
				stack = append(stack, -n-1) // Exit marker
			}
			if enter != nil {
				enter(n)
			}
			g.PerOutEdge(n, func(e Edge) {
				stack = append(stack, e.Dst)
			})
		}
	}
}
