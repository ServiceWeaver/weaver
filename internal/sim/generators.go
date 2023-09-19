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

// TODO(mwhittaker): Add other generators.

// OneOf returns a [Generator] that returns one of the provided values
// equiprobably. OneOf panics if no values are provided.
func OneOf[T any](xs ...T) Generator[T] {
	if len(xs) == 0 {
		panic(fmt.Errorf("OneOf: empty slice"))
	}
	return generatorFunc[T](func(r *rand.Rand) T {
		return xs[r.Intn(len(xs))]
	})
}

// Flip returns a [Generator] that returns true with probability p. Flip
// panics if p is not in the range [0, 1].
func Flip(p float64) Generator[bool] {
	if p < 0 || p > 1 {
		panic(fmt.Errorf("Flip: probability p = %f not in range [0, 1]", p))
	}
	return generatorFunc[bool](func(r *rand.Rand) bool {
		return r.Float64() <= p
	})
}

// Range returns a [Generator] that returns integers equiprobably in the range
// [low, high). Range panics if low >= high.
func Range(low, high int) Generator[int] {
	if low >= high {
		panic(fmt.Errorf("Range: low (%d) >= high (%d)", low, high))
	}
	return generatorFunc[int](func(r *rand.Rand) int {
		delta := high - low
		return low + r.Intn(delta)
	})
}

// generatorFunc[T] is an instance of Generator[T] that uses the provided
// function to generate values.
type generatorFunc[T any] func(r *rand.Rand) T

// Generate implements the Generator interface.
func (g generatorFunc[T]) Generate(r *rand.Rand) T {
	return g(r)
}
