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

package sim

import (
	"fmt"
	"math/rand"
)

// pop pops and returns a randomly selected element from the provided slice.
// pop panics if the provided slice is empty.
func pop[T any](r *rand.Rand, xs []T) (T, []T) {
	if len(xs) == 0 {
		panic(fmt.Errorf("pop: empty slice"))
	}
	i := r.Intn(len(xs))
	x := xs[i]
	return x, append(xs[:i], xs[i+1:]...)
}

// pick returns a randomly selected element from the provided slice. pick
// panics if the provided slice is empty.
func pick[T any](r *rand.Rand, xs []T) T {
	if len(xs) == 0 {
		panic(fmt.Errorf("pick: empty slice"))
	}
	return xs[r.Intn(len(xs))]
}

// flip returns true with probability p. For example, flip(0) always returns
// false, flip(1) always returns true, and flip(0.5) returns true half the
// time. flip panics if p is not in the range [0, 1].
func flip(r *rand.Rand, p float64) bool {
	if p < 0 || p > 1 {
		panic(fmt.Errorf("flip: probability %f not in range [0, 1.0]", p))
	}
	return r.Float64() <= p
}

// ints represents a remove-only set of integers in the range [low, high).
type ints struct {
	low, high int

	// The integers in the set in no particular order.
	elements []int

	// indices[x-low] is the index of element x in elements, or -1 if x is not
	// in the set.
	indices []int
}

// newInts returns a new set of integers in the range [low, high).
// newInts panics if low >= high.
func newInts(low, high int) *ints {
	if low >= high {
		panic(fmt.Errorf("newInts: low (%d) >= high (%d)", low, high))
	}

	n := high - low
	elements := make([]int, n)
	indices := make([]int, n)
	for i := 0; i < n; i++ {
		elements[i] = low + i
		indices[i] = i
	}
	return &ints{low, high, elements, indices}
}

// has returns whether the provided integer is in the set.
func (i *ints) has(x int) bool {
	return i.low <= x && x < i.high && i.indices[x-i.low] != -1
}

// size returns the size of the set.
func (i *ints) size() int {
	return len(i.elements)
}

// pick returns a random element of the set.
func (i *ints) pick(r *rand.Rand) int {
	return i.elements[r.Intn(len(i.elements))]
}

// remove removes the provided element from the set. remove is a noop if the
// provided element is not in the set.
func (i *ints) remove(x int) {
	if !i.has(x) {
		return
	}

	// Swap x with the last element in the set.
	n := len(i.elements)          // number of elements
	j := i.indices[x-i.low]       // index of x
	last := i.elements[n-1]       // last element in the set
	i.elements[j] = last          // move the last element to where x was
	i.elements = i.elements[:n-1] // shrink the slice
	i.indices[last-i.low] = j     // update the last element's index
	i.indices[x-i.low] = -1       // update x's index
}
