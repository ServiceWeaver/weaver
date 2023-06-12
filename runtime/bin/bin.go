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
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"fmt"
	"os"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

// ROData returns the read-only data section of the provided binary.
func ROData(file string) ([]byte, error) {
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
	data, err := ROData(file)
	if err != nil {
		return nil, err
	}
	return codegen.ExtractEdges(data), nil
}

// ReadListeners reads the sets of listeners associated with each component
// in the specified binary.
func ReadListeners(file string) ([]codegen.ComponentListeners, error) {
	data, err := ROData(file)
	if err != nil {
		return nil, err
	}
	return codegen.ExtractListeners(data), nil
}
