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

package version

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExtractVersion(t *testing.T) {
	for _, test := range []struct{ major, minor, patch int }{
		{0, 0, 0},
		{10, 10, 10},
		{123, 4567, 891011},
	} {
		name := fmt.Sprintf("%d.%d.%d", test.major, test.minor, test.patch)
		t.Run(name, func(t *testing.T) {
			// Embed the version string inside a big array of bytes.
			var bytes [10000]byte
			v := fmt.Sprintf("⟦wEaVeRvErSiOn:%d.%d.%d⟧", test.major, test.minor, test.patch)
			copy(bytes[1234:], []byte(v))

			// Extract the version string.
			major, minor, patch, err := extractVersion(bytes[:])
			if err != nil {
				t.Fatal(err)
			}
			if major != test.major {
				t.Errorf("major: got %d, want %d", major, test.major)
			}
			if minor != test.minor {
				t.Errorf("minor: got %d, want %d", minor, test.minor)
			}
			if patch != test.patch {
				t.Errorf("patch: got %d, want %d", patch, test.patch)
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
			err := cmd.Run()
			if err != nil {
				t.Fatal(err)
			}

			// Read version.
			major, minor, patch, err := ReadVersion(binary)
			if err != nil {
				t.Fatal(err)
			}
			if major != Major {
				t.Errorf("major: got %d, want %d", major, Major)
			}
			if minor != Minor {
				t.Errorf("minor: got %d, want %d", minor, Minor)
			}
			if patch != Patch {
				t.Errorf("patch: got %d, want %d", patch, Patch)
			}
		})
	}
}
