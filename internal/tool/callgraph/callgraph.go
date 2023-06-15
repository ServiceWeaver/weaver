// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package callgraph contains code to create visualizations of component
// call graphs stored inside a Service Weaver binary.
package callgraph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ServiceWeaver/weaver/runtime/bin"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"golang.org/x/exp/maps"
)

// Mermaid returns a Mermaid diagram, https://mermaid.js.org/, of the component
// call graph embedded in the provided Service Weaver binary.
func Mermaid(binary string) (string, error) {
	g, err := readGraph(binary)
	if err != nil {
		return "", err
	}
	return g.mermaid(), nil
}

// graph is a (potentially cyclic) call graph.
type graph struct {
	edges []edge
}

// An edge from src to dst exists when component src has a weaver.Ref on dst.
type edge struct {
	src, dst string
}

// readGraph reads a graph from the provided Service Weaver binary.
func readGraph(binary string) (*graph, error) {
	edges, err := bin.ReadComponentGraph(binary)
	if err != nil {
		return nil, fmt.Errorf("read graph %q: %w", binary, err)
	}

	g := graph{}
	for _, e := range edges {
		g.edges = append(g.edges, edge{e[0], e[1]})
	}
	return &g, nil
}

// nodes returns the nodes (aka components) in a graph.
func (g *graph) nodes() []string {
	components := map[string]struct{}{}
	for _, edge := range g.edges {
		components[edge.src] = struct{}{}
		components[edge.dst] = struct{}{}
	}
	keys := maps.Keys(components)
	sort.Strings(keys)
	return keys
}

// mermaid returns a Mermaid diagram of the graph.
func (g *graph) mermaid() string {
	// See https://mermaid.js.org/syntax/flowchart.html for details.
	var b strings.Builder
	fmt.Fprintln(&b, `%%{init: {"flowchart": {"defaultRenderer": "elk"}} }%%`)
	fmt.Fprintln(&b, "graph TD")

	// Nodes.
	fmt.Fprintln(&b, `    %% Nodes.`)
	for _, node := range g.nodes() {
		fmt.Fprintf(&b, "    %s(%s)\n", node, logging.ShortenComponent(node))
	}
	fmt.Fprintln(&b, "")

	// Edges.
	fmt.Fprintln(&b, `    %% Edges.`)
	for _, edge := range g.edges {
		fmt.Fprintf(&b, "    %s --> %s\n", edge.src, edge.dst)
	}
	return b.String()
}

// TODO(mwhittaker): Support graphviz?
