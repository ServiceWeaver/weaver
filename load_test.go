// Copyright 2022 Google LLC
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

package weaver

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func at(seconds int) time.Time {
	var epoch time.Time
	return epoch.Add(time.Duration(seconds) * time.Second)
}

func TestIndex(t *testing.T) {
	assignment := &protos.Assignment{
		Slices: []*protos.Assignment_Slice{
			{Start: 0, Replicas: []string{"a"}},
			{Start: 10, Replicas: []string{"b"}},
			{Start: 20, Replicas: []string{"c"}},
			{Start: 21, Replicas: []string{"d"}},
			{Start: 30, Replicas: []string{"e"}},
		},
	}
	got := newIndex(assignment)
	want := index{
		{0, 10, []string{"a"}, map[string]bool{"a": true}},
		{10, 20, []string{"b"}, map[string]bool{"b": true}},
		{20, 21, []string{"c"}, map[string]bool{"c": true}},
		{21, 30, []string{"d"}, map[string]bool{"d": true}},
		{30, math.MaxUint64, []string{"e"}, map[string]bool{"e": true}},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(slice{})); diff != "" {
		t.Fatalf("bad index (-want +got):\n%s", diff)
	}
}

func TestFindSlice(t *testing.T) {
	assignment := &protos.Assignment{
		Slices: []*protos.Assignment_Slice{
			{Start: 0, Replicas: []string{"a"}},
			{Start: 10, Replicas: []string{"b"}},
			{Start: 20, Replicas: []string{"c"}},
			{Start: 21, Replicas: []string{"d"}},
			{Start: 30, Replicas: []string{"e"}},
		},
	}
	index := newIndex(assignment)
	for _, test := range []struct {
		key  uint64
		want slice
	}{
		{0, slice{0, 10, []string{"a"}, map[string]bool{"a": true}}},
		{1, slice{0, 10, []string{"a"}, map[string]bool{"a": true}}},
		{9, slice{0, 10, []string{"a"}, map[string]bool{"a": true}}},
		{10, slice{10, 20, []string{"b"}, map[string]bool{"b": true}}},
		{11, slice{10, 20, []string{"b"}, map[string]bool{"b": true}}},
		{19, slice{10, 20, []string{"b"}, map[string]bool{"b": true}}},
		{20, slice{20, 21, []string{"c"}, map[string]bool{"c": true}}},
		{21, slice{21, 30, []string{"d"}, map[string]bool{"d": true}}},
		{22, slice{21, 30, []string{"d"}, map[string]bool{"d": true}}},
		{29, slice{21, 30, []string{"d"}, map[string]bool{"d": true}}},
		{30, slice{30, math.MaxUint64, []string{"e"}, map[string]bool{"e": true}}},
		{31, slice{30, math.MaxUint64, []string{"e"}, map[string]bool{"e": true}}},
	} {
		t.Run(fmt.Sprint(test.key), func(t *testing.T) {
			got, ok := index.find(test.key)
			if !ok {
				t.Fatal("slice not found")
			}
			if diff := cmp.Diff(test.want, got, cmp.AllowUnexported(slice{})); diff != "" {
				t.Fatalf("bad slice (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadCollector(t *testing.T) {
	assignment := &protos.Assignment{
		Slices: []*protos.Assignment_Slice{
			{Start: 0, Replicas: []string{"test://a"}},
			{Start: 10, Replicas: []string{"test://b"}},
			{Start: 20, Replicas: []string{"test://a"}},
			{Start: 30, Replicas: []string{"test://b"}},
		},
		Version: 0,
	}
	lc := newLoadCollector("component", "test://a")
	lc.now = func() time.Time { return at(0) }
	lc.updateAssignment(assignment)

	lc.add(0, 1.0)
	lc.add(1, 1.0)
	lc.add(2, 1.0)
	lc.add(3, 1.0)
	for i := 0; i < 10; i++ {
		lc.add(20, 1.0)
		lc.add(21, 1.0)
		lc.add(22, 1.0)
		lc.add(23, 1.0)
		lc.add(24, 1.0)
		lc.add(25, 1.0)
	}

	lc.now = func() time.Time { return at(10) }
	got := lc.report()
	want := &protos.LoadReport_ComponentLoad{
		Load: []*protos.LoadReport_SliceLoad{
			{Start: 0, End: 10, Load: 0.4},
			{Start: 20, End: 30, Load: 6.0},
		},
		Version: 0,
	}
	opts := []cmp.Option{
		protocmp.Transform(),
		protocmp.IgnoreFields(&protos.LoadReport_SliceLoad{}, "size", "splits"),
	}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("bad load report: (-want +got):\n%s", diff)
	}
}

func TestLoadCollectorSizeAndSplitEstimates(t *testing.T) {
	// Test plan: Add load for n different keys. The size estimate should be
	// close to n, but almost certainly isn't exactly n. We check that the size
	// estimate is in the range [0.9*n, 1.1*n].
	//
	// Similarly, the reservoir sample should be representative of the uniform
	// distribution of keys. This is harder to test, but we look at the median
	// of the sample and check that it is close (within 25%) of the true
	// median.

	// Create an assignment where replica a has the slice [0, 1<<20].
	assignment := &protos.Assignment{
		Slices: []*protos.Assignment_Slice{
			{Start: 0, Replicas: []string{"test://a"}},
			{Start: 1 << 20, Replicas: []string{"test://b"}},
		},
		Version: 0,
	}
	lc := newLoadCollector("component", "test://a")
	lc.now = func() time.Time { return at(0) }
	lc.updateAssignment(assignment)

	// Add load for n distinct keys.
	const n = 100 * 1000
	for i := uint64(0); i < n; i++ {
		lc.add(i, 1.0)
	}

	// Check the load.
	lc.now = func() time.Time { return at(10) }
	got := lc.report()
	want := &protos.LoadReport_ComponentLoad{
		Load: []*protos.LoadReport_SliceLoad{
			{Start: 0, End: 1 << 20, Load: n / 10},
		},
		Version: 0,
	}
	opts := []cmp.Option{
		protocmp.Transform(),
		protocmp.IgnoreFields(&protos.LoadReport_SliceLoad{}, "size", "splits"),
	}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("bad load report: (-want +got):\n%s", diff)
	}

	// Test that the estimated size is close to the actual size, n.
	if size := got.Load[0].Size; size < 0.9*n || size > 1.1*n {
		t.Fatalf("bad size estimate: got %d, want %d", size, n)
	}

	// Test that the first split starts at the slice boundary.
	splits := got.Load[0].Splits
	if got, want := got.Load[0].Start, splits[0].Start; got != want {
		t.Fatalf("bad first split start: got %d, want %d", got, want)
	}

	// Test that the split starts are unique and increasing.
	for i := 1; i < len(splits); i++ {
		splita := splits[i-1]
		splitb := splits[i]
		if splita.Start >= splitb.Start {
			t.Fatalf("split %d start (%d) >= split %d start (%d)",
				i-1, splita.Start, i, splitb.Start)
		}
	}

	// Test that the sum of the split loads is equal to the total load.
	sum := 0.0
	for _, split := range splits {
		sum += split.Load
	}
	if !approxEqual(sum, got.Load[0].Load) {
		t.Fatalf("bad sum of split loads: got %f, want %f", sum, got.Load[0].Load)
	}

	// Test that the split loads are all roughly the same.
	mean := got.Load[0].Load / float64(len(splits))
	for _, split := range splits {
		low := 0.75 * mean
		high := 1.25 * mean
		if split.Load < low || split.Load > high {
			t.Fatalf("uneven split load: got %f, want [%f, %f]", split.Load, low, high)
		}
	}

	// Test that the median split is roughly half way in the slice.
	middle := splits[len(splits)/2]
	if middle.Start < 0.75*n/2 || middle.Start > 1.25*n/2 {
		t.Fatalf("bad middle split: got %d, want %d", middle.Start, n/2)
	}
}

func TestSubslices(t *testing.T) {
	for _, test := range []struct {
		load float64
		xs   []uint64
		n    int
		want []*protos.LoadReport_SubsliceLoad
	}{
		// Balanced load, 1 split.
		{
			10.0,
			[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
			1,
			[]*protos.LoadReport_SubsliceLoad{
				{Start: 0, Load: 10.0},
			},
		},
		// Balanced load, 2 splits.
		{
			10.0,
			[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
			2,
			[]*protos.LoadReport_SubsliceLoad{
				{Start: 0, Load: 5.0},
				{Start: 5, Load: 5.0},
			},
		},
		// Balanced load, 3 splits.
		{
			10.0,
			[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
			3,
			[]*protos.LoadReport_SubsliceLoad{
				{Start: 0, Load: 10.0 / 3.0},
				{Start: 3, Load: 10.0 / 3.0},
				{Start: 6, Load: 10.0 / 3.0},
			},
		},
		// Balanced load, 4 splits.
		{
			10.0,
			[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
			4,
			[]*protos.LoadReport_SubsliceLoad{
				{Start: 0, Load: 2.5},
				{Start: 2, Load: 2.5},
				{Start: 5, Load: 2.5},
				{Start: 7, Load: 2.5},
			},
		},
		// Balanced load, 5 splits.
		{
			10.0,
			[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
			5,
			[]*protos.LoadReport_SubsliceLoad{
				{Start: 0, Load: 2.0},
				{Start: 2, Load: 2.0},
				{Start: 4, Load: 2.0},
				{Start: 6, Load: 2.0},
				{Start: 8, Load: 2.0},
			},
		},
		// Skewed load, 5 splits coalesced into 3 splits.
		{
			10.0,
			[]uint64{0, 0, 0, 0, 0, 1, 1, 2, 3, 4},
			5,
			[]*protos.LoadReport_SubsliceLoad{
				{Start: 0, Load: 6.0},
				{Start: 1, Load: 2.0},
				{Start: 3, Load: 2.0},
			},
		},
	} {
		name := fmt.Sprintf("%f/%v/%d", test.load, test.xs, test.n)
		t.Run(name, func(t *testing.T) {
			got := subslices(test.load, test.xs, test.n)
			if diff := cmp.Diff(test.want, got, protocmp.Transform()); diff != "" {
				t.Fatalf("subslices (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPercentiles(t *testing.T) {
	for _, test := range []struct {
		xs   []uint64
		n    int
		want []uint64
	}{
		// Happy cases.
		{[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 1, []uint64{0}},
		{[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 2, []uint64{0, 5}},
		{[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 3, []uint64{0, 3, 6}},
		{[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 4, []uint64{0, 2, 5, 7}},
		{[]uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, 5, []uint64{0, 2, 4, 6, 8}},

		// Fewer points than splits.
		{[]uint64{0}, 5, []uint64{0, 0, 0, 0, 0}},
		{[]uint64{0, 1}, 5, []uint64{0, 0, 0, 1, 1}},
		{[]uint64{0, 1, 2, 3}, 8, []uint64{0, 0, 1, 1, 2, 2, 3, 3}},
	} {
		name := fmt.Sprintf("%v/%d", test.xs, test.n)
		t.Run(name, func(t *testing.T) {
			got := percentiles(test.xs, test.n)
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Fatalf("percentiles (-want +got):\n%s", diff)
			}
		})
	}
}
