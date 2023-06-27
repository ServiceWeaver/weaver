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
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/version"
)

// deployerVersion exists to embed the deployer API version into a Service
// Weaver binary. We split declaring and assigning version to prevent the
// compiler from erasing it.
//
//nolint:unused
var deployerVersion string

func init() {
	// NOTE that deployerVersion must be assigned a string constant that
	// reflects the values of version.DeployerMajor and version.DeployerMinor.
	// If the string is not a constant---if we try to use fmt.Sprintf, for
	// example---it will not be embedded in a Service Weaver binary.
	deployerVersion = "⟦wEaVeRdEpLoYeRvErSiOn:v0.14.0⟧"
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
// binary. The return value is a sequence [src,dst] edges where src and
// dst are fully qualified component names. E.g.,
//
//	github.com/ServiceWeaver/weaver/Main
func ReadComponentGraph(file string) ([][2]string, error) {
	data, err := rodata(file)
	if err != nil {
		return nil, err
	}
	return codegen.ExtractEdges(data), nil
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

// ReadDeployerVersion reads the deployer API version from the specified binary.
func ReadDeployerVersion(filename string) (version.SemVer, error) {
	data, err := rodata(filename)
	if err != nil {
		return version.SemVer{}, err
	}
	return extractDeployerVersion(data)
}

// extractDeployerVersion returns the deployer API version embedded in data.
func extractDeployerVersion(data []byte) (version.SemVer, error) {
	re := regexp.MustCompile(`⟦wEaVeRdEpLoYeRvErSiOn:v([0-9]*?)\.([0-9]*?)\.([0-9]*?)⟧`)
	m := re.FindSubmatch(data)
	if m == nil {
		return version.SemVer{}, fmt.Errorf("embedded deployer API version not found")
	}
	major, minor, patch := string(m[1]), string(m[2]), string(m[3])

	v := version.SemVer{}
	var err error
	v.Major, err = strconv.Atoi(major)
	if err != nil {
		return version.SemVer{}, fmt.Errorf("invalid embedded deployer API major %q: %w", major, err)
	}
	v.Minor, err = strconv.Atoi(minor)
	if err != nil {
		return version.SemVer{}, fmt.Errorf("invalid embedded deployer API minor %q: %w", minor, err)
	}
	v.Patch, err = strconv.Atoi(patch)
	if err != nil {
		return version.SemVer{}, fmt.Errorf("invalid embedded deployer API patch %q: %w", patch, err)
	}
	return v, nil
}
