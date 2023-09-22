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

// Package bin contains code to extract data from a Service Weaver binary.
package bin

import (
	"bytes"
	"debug/buildinfo"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/graph"
	"github.com/ServiceWeaver/weaver/runtime/version"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

// versionData exists to embed the weaver module version and deployer API
// version into a Service Weaver binary. We split declaring and assigning
// versionData to prevent the compiler from erasing it.
//
//lint:ignore U1000 See comment above.
var versionData string

func init() {
	// NOTE that versionData must be assigned a string constant that reflects
	// the value of version.DeployerVersion. If the string is not a
	// constant---if we try to use fmt.Sprintf, for example---it will not be
	// embedded in a Service Weaver binary.
	versionData = "⟦wEaVeRvErSiOn:deployer=v0.22.0⟧"
}

// rodata returns the read-only data section of the provided binary.
func rodata(file string) ([]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Look at first few bytes to determine the file format.
	prefix := make([]byte, 4)
	if _, err := f.ReadAt(prefix, 0); err != nil {
		return nil, err
	}

	// Handle the file formats we support.
	switch {
	case bytes.HasPrefix(prefix, []byte("\x7FELF")): // Linux
		f, err := elf.NewFile(f)
		if err != nil {
			return nil, err
		}
		return f.Section(".rodata").Data()
	case bytes.HasPrefix(prefix, []byte("MZ")): // Windows
		f, err := pe.NewFile(f)
		if err != nil {
			return nil, err
		}
		return f.Section(".rdata").Data()
	case bytes.HasPrefix(prefix, []byte("\xFE\xED\xFA")): // MacOS
		f, err := macho.NewFile(f)
		if err != nil {
			return nil, err
		}
		return f.Section("__rodata").Data()
	case bytes.HasPrefix(prefix[1:], []byte("\xFA\xED\xFE")): // MacOS
		f, err := macho.NewFile(f)
		if err != nil {
			return nil, err
		}
		return f.Section("__rodata").Data()
	default:
		return nil, fmt.Errorf("unknown format")
	}
}

// ReadComponentGraph reads component graph information from the specified
// binary. It returns a slice of components and a component graph whose nodes
// are indices into that slice.
func ReadComponentGraph(file string) ([]string, graph.Graph, error) {
	data, err := rodata(file)
	if err != nil {
		return nil, nil, err
	}
	es := codegen.ExtractEdges(data)

	// Assign node numbers to components in some deterministic order.
	const mainComponent = "github.com/ServiceWeaver/weaver/Main"
	// NOTE: initially, all node numbers are zero.
	nodeMap := map[string]graph.Node{mainComponent: 0}
	for _, e := range es {
		nodeMap[e[0]] = 0
		nodeMap[e[1]] = 0
	}
	// Assign node numbers.
	components := maps.Keys(nodeMap)
	slices.Sort(components)
	for i, c := range components {
		nodeMap[c] = graph.Node(i)
	}

	// Convert component edges into graph edges.
	var edges []graph.Edge
	for _, e := range es {
		src := nodeMap[e[0]]
		dst := nodeMap[e[1]]
		edges = append(edges, graph.Edge{Src: src, Dst: dst})
	}
	return components, graph.NewAdjacencyGraph(maps.Values(nodeMap), edges), nil
}

// ReadListeners reads the sets of listeners associated with each component
// in the specified binary.
func ReadListeners(file string) ([]codegen.ComponentListeners, error) {
	data, err := rodata(file)
	if err != nil {
		return nil, err
	}
	return codegen.ExtractListeners(data), nil
}

type Versions struct {
	ModuleVersion   string         // Service Weaver library's module version
	DeployerVersion version.SemVer // see version.DeployerVersion
}

// ReadVersions reads the module version and deployer API version from the
// specified binary.
func ReadVersions(filename string) (Versions, error) {
	moduleVersion, err := extractModuleVersion(filename)
	if err != nil {
		return Versions{}, err
	}

	data, err := rodata(filename)
	if err != nil {
		return Versions{}, err
	}
	deployerVersion, err := extractDeployerVersion(data)
	if err != nil {
		return Versions{}, err
	}
	return Versions{
		ModuleVersion:   moduleVersion,
		DeployerVersion: deployerVersion,
	}, nil
}

// extractModuleVersion returns the version of the Service Weaver library
// embedded in data.
func extractModuleVersion(filename string) (string, error) {
	info, err := buildinfo.ReadFile(filename)
	if err != nil {
		return "", err
	}

	// Find the Service Weaver module.
	const weaverModule = "github.com/ServiceWeaver/weaver"
	for _, m := range append(info.Deps, &info.Main) {
		if m.Path == weaverModule {
			return m.Version, nil
		}
	}
	return "", fmt.Errorf("Service Weaver module was not linked into the application binary: that's an error")
}

// extractDeployerVersion returns the deployer API version embedded
// in data.
func extractDeployerVersion(data []byte) (version.SemVer, error) {
	re := regexp.MustCompile(`⟦wEaVeRvErSiOn:deployer=v([0-9]*?)\.([0-9]*?)\.([0-9]*?)⟧`)
	m := re.FindSubmatch(data)
	if m == nil {
		return version.SemVer{}, fmt.Errorf("embedded versions not found")
	}
	v := version.SemVer{}
	for _, segment := range []struct {
		name string
		data []byte
		dst  *int
	}{
		{"deployer major version", m[1], &v.Major},
		{"deployer minor version", m[2], &v.Minor},
		{"deployer patch version", m[3], &v.Patch},
	} {
		s := string(segment.data)
		x, err := strconv.Atoi(s)
		if err != nil {
			return version.SemVer{}, fmt.Errorf("invalid embedded %s %q: %w", segment.name, s, err)
		}
		*segment.dst = x
	}
	return v, nil
}
