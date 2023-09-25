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

package graph

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDFSAll(t *testing.T) {
	g := NewAdjacencyGraph(
		[]Node{1, 2, 3, 4},
		[]Edge{
			{2, 1},
			{2, 3},
			{3, 2},
			{3, 4},
		},
	)
	var got []string
	DFSAll(g,
		func(n Node) { got = append(got, fmt.Sprint("enter ", n)) },
		func(n Node) { got = append(got, fmt.Sprint("leave ", n)) })
	want := []string{
		"enter 1",
		"leave 1",
		"enter 2",
		"enter 3",
		"enter 4",
		"leave 4",
		"leave 3",
		"leave 2",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("DFSAll for:\n%s\nDiff (-want +got):\n%s",
			DebugString(g), diff)
	}
}
