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
	"math"
	"math/rand"
)

// TODO(mwhittaker): Add more generators as needed.

// Booleans

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

// Numerics

// NonNegativeInt returns a [Generator] that returns non-negative integers.
// Note that NonNegativeInt does not return all numbers. Instead, it biases
// towards numbers closer to zero and other pathological numbers that are more
// likely to induce bugs (e.g., math.MaxInt).
func NonNegativeInt() Generator[int] {
	choices := []Weighted[int]{
		{100, Range(0, 10)},
		{100, Range(10, 100)},
		{100, Range(100, 1000)},
		{100, Range(1000, 10_000)},
		{100, Range(10_000, 100_000)},
		{100, Range(100_000, 1_000_000)},
		{100, Range(1_000_000, 1_000_000_000)},
		{100, Range(1_000_000_000, 1_000_000_000_000)},
		{100, Range(1_000_000_000_000, math.MaxInt)},
		{1, OneOf(
			math.MaxInt, math.MaxInt-1,
			math.MaxInt32+1, math.MaxInt32, math.MaxInt32-1,
			math.MaxInt16+1, math.MaxInt16, math.MaxInt16-1,
			math.MaxInt8+1, math.MaxInt8, math.MaxInt8-1,
		)},
	}
	return Weight(choices)
}

// Int returns a [Generator] that returns integers. Note that Int does not
// return all integers equiprobably. Instead, it biases towards numbers closer
// to zero and other pathological numbers that are more likely to induce bugs
// (e.g., math.MaxInt, math.MinInt).
func Int() Generator[int] {
	choices := []Weighted[int]{
		{100, Range(-10, 10)},
		{50, Range(10, 100)},
		{50, Range(-100, -10)},
		{50, Range(100, 1000)},
		{50, Range(-1000, -100)},
		{50, Range(1000, 10_000)},
		{50, Range(-10_000, -1000)},
		{50, Range(10_000, 100_000)},
		{50, Range(-100_000, -10_000)},
		{50, Range(100_000, 1_000_000)},
		{50, Range(-1_000_000, -100_000)},
		{50, Range(1_000_000, 1_000_000_000)},
		{50, Range(-1_000_000_000, -1_000_000)},
		{50, Range(1_000_000_000, 1_000_000_000_000)},
		{50, Range(-1_000_000_000_000, -1_000_000_000)},
		{50, Range(1_000_000_000_000, math.MaxInt)},
		{50, Range(math.MinInt, -1_000_000_000_000)},
		{1, OneOf(
			math.MaxInt, math.MaxInt-1,
			math.MinInt, math.MinInt+1,
			math.MaxInt32+1, math.MaxInt32, math.MaxInt32-1,
			math.MinInt32-1, math.MinInt32, math.MinInt32+1,
			math.MaxInt16+1, math.MaxInt16, math.MaxInt16-1,
			math.MinInt16-1, math.MinInt16, math.MinInt16+1,
			math.MaxInt8+1, math.MaxInt8, math.MaxInt8-1,
			math.MinInt8-1, math.MinInt8, math.MinInt8+1,
		)},
	}
	return Weight(choices)
}

// Float64 returns a [Generator] that returns 64-bit floats. Note that Float64
// does not return all floats equiprobably. Instead, it biases towards numbers
// closer to zero and other pathological numbers that are more likely to induce
// bugs (e.g., NaN, infinity, -infinity, -0).
func Float64() Generator[float64] {
	pathologies := OneOf(
		math.NaN(),           // NaN
		math.Inf(1),          // infinity
		math.Inf(-1),         // -infinity
		math.Copysign(0, -1), // -0
	)
	return generatorFunc[float64](func(r *rand.Rand) float64 {
		if r.Intn(1000) == 0 {
			// 0.1% of the time, return a pathological float.
			return pathologies.Generate(r)
		}
		const stdev = 1e6
		return r.NormFloat64() * stdev
	})
}

// Rune returns a [Generator] that returns runes equiprobably.
func Rune() Generator[rune] {
	return generatorFunc[rune](func(r *rand.Rand) rune {
		// Note that rune is an alias for int32.
		return rune(r.Intn(math.MaxInt32 + 1))
	})
}

// Byte returns a [Generator] that returns bytes equiprobably.
func Byte() Generator[byte] {
	return generatorFunc[byte](func(r *rand.Rand) byte {
		var buf [1]byte
		r.Read(buf[:])
		return buf[0]
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

// Strings

// String returns a [Generator] that returns moderately sized readable strings,
// with a bias towards smaller strings.
func String() Generator[string] {
	// Prefer smaller strings.
	size := Weight([]Weighted[int]{
		{100, Range(0, 10)},
		{10, Range(10, 100)},
		{1, Range(100, 4096)},
	})
	alphabet := OneOf(
		// Lowercase letters.
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
		// Uppercase letters.
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
		// Lowercase Greek letters.
		'α', 'β', 'γ', 'δ', 'ε', 'ζ', 'η', 'θ', 'ι', 'κ', 'λ', 'μ', 'ν', 'ξ', 'ο', 'π', 'ρ', 'σ', 'τ', 'υ', 'φ', 'χ', 'ψ', 'ω',
		// Numbers.
		'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		// Punctuation.
		' ', '-', '_', '.',
	)
	runes := Slice(size, alphabet)
	return generatorFunc[string](func(r *rand.Rand) string {
		return string(runes.Generate(r))
	})
}

// Slices and Maps

// Slice returns a [Generator] that returns slices of T. The size and contents
// of the generated slices are determined by the provided generators.
func Slice[T any](size Generator[int], values Generator[T]) Generator[[]T] {
	return generatorFunc[[]T](func(r *rand.Rand) []T {
		n := size.Generate(r)
		if n < 0 {
			panic(fmt.Errorf("Slice: negative size %d", n))
		}
		xs := make([]T, n)
		for i := 0; i < n; i++ {
			xs[i] = values.Generate(r)
		}
		return xs
	})
}

// Map returns a [Generator] that returns maps from K to V. The size and
// contents of the the generated maps are determined by the provided
// generators.
func Map[K comparable, V any](size Generator[int], keys Generator[K], values Generator[V]) Generator[map[K]V] {
	return generatorFunc[map[K]V](func(r *rand.Rand) map[K]V {
		n := size.Generate(r)
		if n < 0 {
			panic(fmt.Errorf("Map: negative size %d", n))
		}
		kvs := make(map[K]V, n)
		for i := 0; i < n; i++ {
			kvs[keys.Generate(r)] = values.Generate(r)
		}
		return kvs
	})
}

// Combinators

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

// Weight returns a [Generator] that generates values using the provided
// generators. A generator is chosen with probability proportional to its
// weight. For example, given the following choices:
//
//   - Weighted{1.0, OneOf("a")}
//   - Weighted{2.0, OneOf("b")}
//
// Weight returns "b" twice as often as it returns "a". Note that the provided
// weights do not have to sum to 1.
//
// Weight panics if no choices are provided, if any weight is negative, or if
// the sum of all weight is 0.
func Weight[T any](choices []Weighted[T]) Generator[T] {
	if len(choices) == 0 {
		panic(fmt.Errorf("Weight: empty choices"))
	}

	// Compute the sum of all weights.
	total := 0.0
	for _, choice := range choices {
		if choice.Weight < 0 {
			panic(fmt.Errorf("Weight: negative weight %f", choice.Weight))
		}
		total += choice.Weight
	}
	if total == 0 {
		panic(fmt.Errorf("Weight: zero total weight"))
	}

	// Normalize the weights by the total.
	normalized := make([]Weighted[T], len(choices))
	for i, choice := range choices {
		choice.Weight = choice.Weight / total
		normalized[i] = choice
	}

	return generatorFunc[T](func(r *rand.Rand) T {
		f := r.Float64()
		running := 0.0
		for i, choice := range normalized {
			running += choice.Weight
			if f <= running || i == len(normalized)-1 {
				return choice.Gen.Generate(r)
			}
		}
		panic("unreachable")
	})
}

type Weighted[T any] struct {
	Weight float64
	Gen    Generator[T]
}

// Filter returns a [Generator] that returns values from the provided generator
// that satisfy the provided predicate.
func Filter[T any](gen Generator[T], predicate func(T) bool) Generator[T] {
	return generatorFunc[T](func(r *rand.Rand) T {
		for {
			x := gen.Generate(r)
			if predicate(x) {
				return x
			}
		}
	})
}

// generatorFunc[T] is an instance of Generator[T] that uses the provided
// function to generate values.
type generatorFunc[T any] func(r *rand.Rand) T

// Generate implements the Generator interface.
func (g generatorFunc[T]) Generate(r *rand.Rand) T {
	return g(r)
}
