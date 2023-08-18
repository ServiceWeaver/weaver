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
	"slices"

	"golang.org/x/exp/constraints"
	"golang.org/x/exp/maps"
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

// pickKey returns a randomly selected key from the provided map. pickKey
// panics if the provided map is empty.
func pickKey[K constraints.Ordered, V any](r *rand.Rand, kvs map[K]V) K {
	if len(kvs) == 0 {
		panic(fmt.Errorf("pickKey: empty map"))
	}
	keys := maps.Keys(kvs)
	// Sort the keys to make sure pickKey is deterministic.
	slices.Sort(keys)
	return pick(r, keys)
}

// pickValue returns a randomly selected value from the provided map. pickValue
// panics if the provided map is empty.
func pickValue[K constraints.Ordered, V any](r *rand.Rand, kvs map[K]V) V {
	if len(kvs) == 0 {
		panic(fmt.Errorf("pickValue: empty map"))
	}
	return kvs[pickKey(r, kvs)]
}
