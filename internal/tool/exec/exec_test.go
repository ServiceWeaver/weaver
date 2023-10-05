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

package exec

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseTopology(t *testing.T) {
	contents := `
		id = "v1"

		[nodes.main]
		address = "localhost:9000"
		components = ["collatz.Main"]
		listeners.collatz = "localhost:8000"

		[nodes.odd]
		address = "localhost:9001"
		components = ["collatz.Odd"]

		[nodes.even]
		address = ":9002"
		dial_address = "localhost:9002"
		components = ["collatz.Even"]
	`

	want := topology{
		DeploymentId: "v1",
		Nodes: map[string]node{
			"main": {
				Address:    "localhost:9000",
				Components: []string{"collatz.Main"},
				Listeners:  map[string]string{"collatz": "localhost:8000"},
			},
			"odd": {
				Address:    "localhost:9001",
				Components: []string{"collatz.Odd"},
			},
			"even": {
				Address:     ":9002",
				DialAddress: "localhost:9002",
				Components:  []string{"collatz.Even"},
			},
		},
	}

	got, err := parseTopology(contents)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("topology (-want, +got):\n%s", diff)
	}
}
