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
	"io"
	"os"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

// ReadComponentGraph reads component graph information from the specified
// binary. The return value is a sequence [src,dst] edges where src and
// dst are fully qualified component names. E.g.,
//
//	github.com/ServiceWeaver/weaver/Main
func ReadComponentGraph(file string) ([][2]string, error) {
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
	var result [][2]string
	switch {
	case bytes.HasPrefix(prefix, []byte("\x7FELF")): // Linux
		result, err = readElf(f)
	case bytes.HasPrefix(prefix, []byte("MZ")): // Windows
		result, err = readPe(f)
	case bytes.HasPrefix(prefix, []byte("\xFE\xED\xFA")): // MacOS
		result, err = readMacho(f)
	case bytes.HasPrefix(prefix[1:], []byte("\xFA\xED\xFE")): // MacOS
		result, err = readMacho(f)
	default:
		err = fmt.Errorf("unknown format")
	}
	if err != nil {
		return nil, fmt.Errorf("file %s: %w", file, err)
	}
	return result, nil
}

func readElf(in io.ReaderAt) ([][2]string, error) {
	f, err := elf.NewFile(in)
	if err != nil {
		return nil, err
	}
	return searchSection(f.Section(".rodata"))
}

func readPe(in io.ReaderAt) ([][2]string, error) {
	f, err := pe.NewFile(in)
	if err != nil {
		return nil, err
	}
	return searchSection(f.Section(".rdata"))
}

func readMacho(in io.ReaderAt) ([][2]string, error) {
	f, err := macho.NewFile(in)
	if err != nil {
		return nil, err
	}
	return searchSection(f.Section("__rodata"))
}

type section interface {
	Open() io.ReadSeeker
}

func searchSection(s section) ([][2]string, error) {
	if s == nil {
		return nil, fmt.Errorf("read-only data section not found")
	}
	data, err := io.ReadAll(s.Open())
	if err != nil {
		return nil, err
	}
	return codegen.ExtractEdges(data), nil
}
