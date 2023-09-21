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

package bin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/graph"
	"github.com/ServiceWeaver/weaver/runtime/version"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/exp/slices"
)

func TestReadComponentGraph(t *testing.T) {
	for _, test := range []struct{ os, arch string }{
		{"linux", "amd64"},
		{"windows", "amd64"},
		{"darwin", "arm64"},
	} {
		t.Run(fmt.Sprintf("%s/%s", test.os, test.arch), func(t *testing.T) {
			// Build the binary for os/arch.
			d := t.TempDir()
			binary := filepath.Join(d, "bin")
			cmd := exec.Command("go", "build", "-o", binary, "./testprogram")
			cmd.Env = append(os.Environ(), "GOOS="+test.os, "GOARCH="+test.arch)
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}

			// Read the component graph.
			gotComponents, g, err := ReadComponentGraph(binary)
			if err != nil {
				t.Fatal(err)
			}

			// Compare returned components.
			pkg := "github.com/ServiceWeaver/weaver/runtime/bin/testprogram"
			main := "github.com/ServiceWeaver/weaver/Main"
			wantComponents := []string{
				main,
				fmt.Sprintf("%s/A", pkg),
				fmt.Sprintf("%s/B", pkg),
				fmt.Sprintf("%s/C", pkg),
			}
			if diff := cmp.Diff(wantComponents, gotComponents); diff != "" {
				t.Fatalf("unexpected components: (-want +got): %s", diff)
			}

			// Collect nodes from the graph and compare.
			var nodes []graph.Node
			g.PerNode(func(n graph.Node) {
				nodes = append(nodes, n)
			})
			slices.Sort(nodes)
			if diff := cmp.Diff([]graph.Node{0, 1, 2, 3}, nodes); diff != "" {
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
			want := []graph.Edge{
				{Src: 0, Dst: 1},
				{Src: 1, Dst: 2},
				{Src: 1, Dst: 3}}
			if diff := cmp.Diff(want, edges); diff != "" {
				t.Fatalf("unexpected edges: (-want +got): %s", diff)
			}
		})
	}
}

func TestReadListeners(t *testing.T) {
	for _, test := range []struct{ os, arch string }{
		{"linux", "amd64"},
		{"windows", "amd64"},
		{"darwin", "arm64"},
	} {
		t.Run(fmt.Sprintf("%s/%s", test.os, test.arch), func(t *testing.T) {
			// Build the binary for os/arch.
			d := t.TempDir()
			binary := filepath.Join(d, "bin")
			cmd := exec.Command("go", "build", "-o", binary, "./testprogram")
			cmd.Env = append(os.Environ(), "GOOS="+test.os, "GOARCH="+test.arch)
			err := cmd.Run()
			if err != nil {
				t.Fatal(err)
			}

			// Read listeners.
			actual, err := ReadListeners(binary)
			if err != nil {
				t.Fatal(err)
			}

			// Check that expected listeners are found.
			pkg := func(c string) string {
				return fmt.Sprintf("github.com/ServiceWeaver/weaver/runtime/bin/testprogram/%s", c)
			}
			main := "github.com/ServiceWeaver/weaver/Main"
			want := []codegen.ComponentListeners{
				{Component: main, Listeners: []string{"appLis"}},
				{Component: pkg("A"), Listeners: []string{"aLis1", "aLis2", "aLis3"}},
				{Component: pkg("B"), Listeners: []string{"Listener"}},
				{Component: pkg("C"), Listeners: []string{"cLis"}},
			}
			if diff := cmp.Diff(want, actual); diff != "" {
				t.Fatalf("unexpected listeners (-want +got):\n%s", diff)
			}
		})
	}
}

func TestExtractVersion(t *testing.T) {
	for _, want := range []version.SemVer{
		{Major: 4, Minor: 5, Patch: 6},
		{Major: 0, Minor: 567, Patch: 8910},
	} {
		t.Run(want.String(), func(t *testing.T) {
			// Embed the version string inside a big array of bytes.
			var bytes [10000]byte
			embedded := fmt.Sprintf("⟦wEaVeRvErSiOn:deployer=%s⟧", want)
			copy(bytes[1234:], []byte(embedded))

			// Extract the version string.
			got, err := extractDeployerVersion(bytes[:])
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("bad deployer API version: got %s, want %s", got, want)
			}
		})
	}
}

func TestReadVersion(t *testing.T) {
	for _, test := range []struct{ os, arch string }{
		{"linux", "amd64"},
		{"windows", "amd64"},
		{"darwin", "arm64"},
	} {
		t.Run(fmt.Sprintf("%s/%s", test.os, test.arch), func(t *testing.T) {
			// Build the binary for os/arch.
			d := t.TempDir()
			binary := filepath.Join(d, "bin")
			cmd := exec.Command("go", "build", "-o", binary, "./testprogram")
			cmd.Env = append(os.Environ(), "GOOS="+test.os, "GOARCH="+test.arch)
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}

			// Read version.
			got, err := ReadVersions(binary)
			if err != nil {
				t.Fatal(err)
			}

			// NOTE(spetrovic): Test binaries are built without module
			// information, so we can't read the expected module value from
			// the running test binary. For that reason, we only test that
			// the testprogram module version is not empty.
			if got.ModuleVersion == "" {
				t.Fatalf("empty module version")
			}
			if got.DeployerVersion != version.DeployerVersion {
				t.Fatalf("bad deployer version: got %s, want %s", got.DeployerVersion, version.DeployerVersion)
			}
		})
	}
}
