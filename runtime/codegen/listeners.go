// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
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
	"strings"
)

// All listeners used by a given component are embedded in the generated
// binary as specially formatted strings. These strings can be extracted from
// the binary to get the list of listeners associated with each component
// without having to execute the binary.
//
// The set of listeners used by a given component is represented by a string
// fragment that looks like:
// ⟦checksum:wEaVeRlIsTeNeRs:component→listeners⟧
//
// checksum is the first 8 bytes of the hex encoding of the SHA-256 of
// the string "wEaVeRlIsTeNeRs:component→listeners"; component is the fully
// qualified component type name; listeners is a comma-separated list of
// all listener names associated with a given component.

// MakeListenersString returns a string that should be emitted into generated
// code to represent the set of listeners associated with a given component.
func MakeListenersString(component string, listeners []string) string {
	sort.Strings(listeners) // generate a stable encoding
	lisstr := strings.Join(listeners, ",")
	return fmt.Sprintf("⟦%s:wEaVeRlIsTeNeRs:%s→%s⟧\n",
		checksumListeners(component, lisstr), component, lisstr)
}

// ComponentListeners represents a set of listeners for a given component.
type ComponentListeners struct {
	// Fully qualified component type name, e.g.,
	//   github.com/ServiceWeaver/weaver/Main.
	Component string

	// The list of listener names associated with the component.
	Listeners []string
}

// ExtractListeners returns the components and their listeners encoded using
// MakeListenersString() in data.
func ExtractListeners(data []byte) []ComponentListeners {
	var results []ComponentListeners
	re := regexp.MustCompile(`⟦([0-9a-fA-F]+):wEaVeRlIsTeNeRs:([a-zA-Z0-9\-.~_/]*?)→([\p{L}\p{Nd}_,]+)⟧`)
	for _, m := range re.FindAllSubmatch(data, -1) {
		if len(m) != 4 {
			continue
		}
		sum, component, lisstr := string(m[1]), string(m[2]), string(m[3])
		if sum != checksumListeners(component, lisstr) {
			continue
		}
		results = append(results, ComponentListeners{
			Component: component,
			Listeners: strings.Split(lisstr, ","),
		})
	}
	// Generate a stable list.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Component < results[j].Component
	})
	return results
}

func checksumListeners(component, lisstr string) string {
	str := fmt.Sprintf("wEaVeRlIsTeNeRs:%s→%s", component, lisstr)
	sum := sha256.Sum256([]byte(str))
	return fmt.Sprintf("%0x", sum)[:8]
}
