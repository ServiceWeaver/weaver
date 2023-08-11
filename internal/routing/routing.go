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

// Package routing includes utilities for routing and assignments. See
// https://serviceweaver.dev/docs.html#routing for more information on routing.
package routing

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// FormatAssignment pretty formats the provided assignment.
func FormatAssignment(a *protos.Assignment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Assignment Version %d\n", a.Version)
	if len(a.Slices) == 0 {
		return b.String()
	}

	hex := func(x uint64) string {
		return fmt.Sprintf("0x%016x", x)
	}
	for i, si := range a.Slices[:len(a.Slices)-1] {
		sj := a.Slices[i+1]
		fmt.Fprintf(&b, "[%s, %s): %v\n", hex(si.Start), hex(sj.Start), si.Replicas)
	}
	slast := a.Slices[len(a.Slices)-1]
	fmt.Fprintf(&b, "[%s, %s]: %v\n", hex(slast.Start), hex(math.MaxUint64), slast.Replicas)
	return b.String()
}

// EqualSlices returns an assignment with slices of roughly equal size.
// Replicas are assigned to slices in a round robin fashion. The returned
// assignment has a version of 0.
func EqualSlices(replicas []string) *protos.Assignment {
	if len(replicas) == 0 {
		return &protos.Assignment{}
	}

	// Note that the replicas should be sorted. This is required because we
	// want to do a deterministic assignment of slices to replicas among
	// different invocations, to avoid unnecessary churn while generating new
	// assignments.
	replicas = slices.Clone(replicas)
	sort.Strings(replicas)

	// Form n roughly equally sized slices, where n is the least power of two
	// larger than the number of replicas.
	//
	// TODO(mwhittaker): Shouldn't we pick a number divisible by the number of
	// replicas? Otherwise, not every replica gets the same number of slices.
	n := nextPowerOfTwo(len(replicas))
	slices := make([]*protos.Assignment_Slice, n)
	start := uint64(0)
	delta := math.MaxUint64 / uint64(n)
	for i := 0; i < n; i++ {
		slices[i] = &protos.Assignment_Slice{Start: start}
		start += delta
	}

	// Assign replicas to slices in a round robin fashion.
	for i, slice := range slices {
		slice.Replicas = []string{replicas[i%len(replicas)]}
	}
	return &protos.Assignment{Slices: slices}
}

// nextPowerOfTwo returns the least power of 2 that is greater or equal to x.
func nextPowerOfTwo(x int) int {
	switch {
	case x == 0:
		return 1
	case x&(x-1) == 0:
		// x is already power of 2.
		return x
	default:
		return int(math.Pow(2, math.Ceil(math.Log2(float64(x)))))
	}
}
