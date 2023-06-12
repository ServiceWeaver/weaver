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

package codegen

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
)

// Component graph edges are embedded in the generated binary as
// specially formatted strings. These strings can be extracted from
// the binary to get the communication graph without having to execute
// the binary.
//
// Each edge is represented by a string fragment that looks like:
// ⟦checksum:wEaVeReDgE:src→dst⟧
//
// checksum is the first 8 bytes of the hex encoding of the SHA-256 of
// the string "wEaVeReDgE:src→dst" and src and dst are the fully qualified
// component type names.

// MakeEdgeString returns a string that should be emitted into generated
// code to represent an edge from src to dst.
func MakeEdgeString(src, dst string) string {
	return fmt.Sprintf("⟦%s:wEaVeReDgE:%s→%s⟧\n", checksumEdge(src, dst), src, dst)
}

// ExtractEdges returns the edges corresponding to MakeEdgeString() results
// that occur in data.
func ExtractEdges(data []byte) [][2]string {
	var result [][2]string
	re := regexp.MustCompile(`⟦([0-9a-fA-F]+):wEaVeReDgE:([a-zA-Z0-9\-.~_/]*?)→([a-zA-Z0-9\-.~_/]*?)⟧`)
	for _, m := range re.FindAllSubmatch(data, -1) {
		if len(m) != 4 {
			continue
		}
		sum, src, dst := string(m[1]), string(m[2]), string(m[3])
		if sum != checksumEdge(src, dst) {
			continue
		}
		result = append(result, [2]string{src, dst})
	}
	sort.Slice(result, func(i, j int) bool {
		if a, b := result[i][0], result[j][0]; a != b {
			return a < b
		}
		return result[i][1] < result[j][1]
	})
	return result
}

func checksumEdge(src, dst string) string {
	edge := fmt.Sprintf("wEaVeReDgE:%s→%s", src, dst)
	sum := sha256.Sum256([]byte(edge))
	return fmt.Sprintf("%0x", sum)[:8]
}
