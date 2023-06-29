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
	"testing"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/version"
	"github.com/google/go-cmp/cmp"
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

			// Read edges.
			edges, err := ReadComponentGraph(binary)
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			for _, edge := range edges {
				t.Logf("edge %v", edge)
				found[fmt.Sprintf("%s=>%s", edge[0], edge[1])] = true
			}

			// Check that expected edges are found.
			pkg := "github.com/ServiceWeaver/weaver/runtime/bin/testprogram"
			main := "github.com/ServiceWeaver/weaver/Main"
			for _, want := range []string{
				fmt.Sprintf("%s=>%s/A", main, pkg),
				fmt.Sprintf("%s/A=>%s/B", pkg, pkg),
				fmt.Sprintf("%s/A=>%s/C", pkg, pkg),
			} {
				if !found[want] {
					t.Errorf("did not find expected edge %q", want)
				}
			}

			// Check that other edges are not found.
			for _, badedge := range []string{
				fmt.Sprintf("%s/B=>%s/A", pkg, pkg),
				fmt.Sprintf("%s/B=>%s/C", pkg, pkg),
			} {
				if found[badedge] {
					t.Errorf("found unexpected edge %q", badedge)
				}

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
	for _, want := range []Versions{
		{
			ModuleVersion:   version.SemVer{Major: 1, Minor: 2, Patch: 3},
			DeployerVersion: version.SemVer{Major: 4, Minor: 5, Patch: 6},
		},
		{
			ModuleVersion:   version.SemVer{Major: 100, Minor: 100, Patch: 234},
			DeployerVersion: version.SemVer{Major: 0, Minor: 567, Patch: 8910},
		},
	} {
		name := fmt.Sprintf("%s-%s", want.ModuleVersion, want.DeployerVersion)
		t.Run(name, func(t *testing.T) {
			// Embed the version string inside a big array of bytes.
			var bytes [10000]byte
			embedded := fmt.Sprintf("⟦wEaVeRvErSiOn:module=%s;deployer=%s⟧", want.ModuleVersion, want.DeployerVersion)
			copy(bytes[1234:], []byte(embedded))

			// Extract the version string.
			got, err := extractVersions(bytes[:])
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
			if got.ModuleVersion != version.ModuleVersion {
				t.Fatalf("bad module version: got %s, want %s", got.ModuleVersion, version.ModuleVersion)
			}
			if got.DeployerVersion != version.DeployerVersion {
				t.Fatalf("bad deployer version: got %s, want %s", got.DeployerVersion, version.DeployerVersion)
			}
		})
	}
}
