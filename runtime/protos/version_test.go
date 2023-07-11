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

package protos

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"testing"
)

// TestDeployerVersion is designed to make it hard to forget to update the
// deployer API version when changes are made to the deployer API. Concretely,
// TestDeployerVersion detects any changes to runtime.proto. If the file does
// change, the test fails and reminds you to update the deployer API version.
//
// This is tedious, but deployer API changes should be rare and should be
// increasingly rare as time goes on. Plus, forgetting to update the deployer
// API version can be very annoying to users.
func TestDeployerVersion(t *testing.T) {
	// Compute the SHA-256 hash of runtime.proto.
	f, err := os.Open("runtime.proto")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%x", h.Sum(nil))

	// If runtime.proto has changed, the deployer API version may need updating.
	const want = "d81891df353242d5484fbd628eccec29e5cf43f9f78daaba4b00239560dd8e03"
	if got != want {
		t.Fatalf(`Unexpected SHA-256 hash of runtime.proto: got %s, want %s. If this change is meaningful, REMEMBER TO UPDATE THE DEPLOYER API VERSION in runtime/version/version.go.`, got, want)
	}
}
