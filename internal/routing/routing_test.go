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

package routing

import (
	"fmt"
	"testing"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func ExampleFormatAssignment() {
	assignment := &protos.Assignment{
		Slices: []*protos.Assignment_Slice{
			{Start: 0x0, Replicas: []string{"a", "b"}},
			{Start: 0x3333333333333333, Replicas: []string{"a", "c"}},
			{Start: 0x6666666666666666, Replicas: []string{"b", "c"}},
			{Start: 0x9999999999999999, Replicas: []string{"a", "b", "c"}},
		},
		Version: 42,
	}
	fmt.Println(FormatAssignment(assignment))

	// OUTPUT:
	// Assignment Version 42
	// [0x0000000000000000, 0x3333333333333333): [a b]
	// [0x3333333333333333, 0x6666666666666666): [a c]
	// [0x6666666666666666, 0x9999999999999999): [b c]
	// [0x9999999999999999, 0xffffffffffffffff]: [a b c]
}

func TestFormatAssignmentOnAssignmentWithNoSlices(t *testing.T) {
	assignment := &protos.Assignment{Version: 42}
	got := FormatAssignment(assignment)
	const want = "Assignment Version 42\n"
	if got != want {
		t.Fatalf("FormatAssignment: got %q, want %q", got, want)
	}
}

func TestEqualSlicesNoReplicas(t *testing.T) {
	got := EqualSlices([]string{})
	want := &protos.Assignment{}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Fatalf("EqualSlices: (-want +got):\n%s", diff)
	}
}

func TestEqualSlicesOneReplica(t *testing.T) {
	got := EqualSlices([]string{"a"})
	want := &protos.Assignment{
		Slices: []*protos.Assignment_Slice{
			{Start: 0, Replicas: []string{"a"}},
		},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Fatalf("EqualSlices: (-want +got):\n%s", diff)
	}
}

func TestEqualSlicesThreeReplicas(t *testing.T) {
	// - There are 3 replicas.
	// - The least power of 2 larger than 3 is 4, so we create 4 slices.
	// - The slice width is math.MaxUint64 / 4 = 4611686018427387903.75, which
	//   is rounded down to 4611686018427387903.
	// - 2 * 4611686018427387903 = 9223372036854775806.
	// - 3 * 4611686018427387903 = 13835058055282163709.
	got := EqualSlices([]string{"a", "b", "c"})
	want := &protos.Assignment{
		Slices: []*protos.Assignment_Slice{
			{Start: 0, Replicas: []string{"a"}},
			{Start: 4611686018427387903, Replicas: []string{"b"}},
			{Start: 9223372036854775806, Replicas: []string{"c"}},
			{Start: 13835058055282163709, Replicas: []string{"a"}},
		},
	}
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Fatalf("EqualSlices: (-want +got):\n%s", diff)
	}
}

func TestNextPowerOfTwo(t *testing.T) {
	for _, test := range []struct{ x, want int }{
		{0, 1}, {1, 1},
		{2, 2},
		{3, 4}, {4, 4},
		{5, 8}, {6, 8}, {7, 8}, {8, 8},
		{9, 16}, {10, 16}, {11, 16}, {12, 16}, {13, 16}, {14, 16}, {15, 16}, {16, 16},
	} {
		if got, want := nextPowerOfTwo(test.x), test.want; got != want {
			t.Errorf("nextPowerOfTwo(%d): got %d, want %d", test.x, got, want)
		}
	}
}
