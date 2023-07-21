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

	"golang.org/x/exp/constraints"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

func pop[T any](r *rand.Rand, xs []T) (T, []T) {
	if len(xs) == 0 {
		panic(fmt.Errorf("pop: no options given"))
	}
	i := r.Intn(len(xs))
	x := xs[i]
	return x, append(xs[:i], xs[i+1:]...)
}

func pick[T any](r *rand.Rand, xs []T) T {
	if len(xs) == 0 {
		panic(fmt.Errorf("pick: no choices"))
	}
	return xs[r.Intn(len(xs))]
}

func pickKey[K constraints.Ordered, V any](r *rand.Rand, kvs map[K]V) K {
	if len(kvs) == 0 {
		panic(fmt.Errorf("pickKey: no choices"))
	}
	keys := maps.Keys(kvs)
	slices.Sort(keys)
	return pick(r, keys)
}

func pickValue[K constraints.Ordered, V any](r *rand.Rand, kvs map[K]V) V {
	return kvs[pickKey(r, kvs)]
}
