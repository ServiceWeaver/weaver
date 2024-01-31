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
	"math/rand"
	"testing"
	"time"
	"unicode"
)

type frequency[T any] struct {
	fraction float64
	x        T
}

// testGenerator samples a large number of values from the provided generator
// and checks that the distribution of values is close to the expected
// distribution.
func testGenerator[T comparable](t *testing.T, gen Generator[T], wants []frequency[T]) {
	// Sample gen.
	r := rand.New(rand.NewSource(0))
	const n = 10_000_000
	got := map[T]int{}
	for i := 0; i < n; i++ {
		got[gen.Generate(r)]++
	}

	// Check that the distribution of is close to what's expected.
	for _, want := range wants {
		gotFraction := float64(got[want.x]) / n
		wantFraction := want.fraction
		slack := wantFraction / 10
		if gotFraction < wantFraction-slack || gotFraction > wantFraction+slack {
			t.Errorf("%v fraction: got %f, want in [%f, %f]", want.x, gotFraction, wantFraction-slack, wantFraction+slack)
		}
	}
}

func TestFlip(t *testing.T) {
	const p = 0.1
	gen := Flip(p)
	wants := []frequency[bool]{{p, true}, {1 - p, false}}
	testGenerator(t, gen, wants)
}

func TestByte(t *testing.T) {
	wants := make([]frequency[byte], 256)
	for i := 0; i < 256; i++ {
		wants[i] = frequency[byte]{1. / 256, byte(i)}
	}
	testGenerator(t, Byte(), wants)
}

func TestRange(t *testing.T) {
	gen := Range(0, 4)
	wants := []frequency[int]{{0.25, 0}, {0.25, 1}, {0.25, 2}, {0.25, 3}}
	testGenerator(t, gen, wants)
}

func TestOneOf(t *testing.T) {
	gen := OneOf("a", "b", "c", "d")
	wants := []frequency[string]{{0.25, "a"}, {0.25, "b"}, {0.25, "c"}, {0.25, "d"}}
	testGenerator(t, gen, wants)
}

func TestWeight(t *testing.T) {
	gen := Weight([]Weighted[string]{
		{50, OneOf("a")},
		{25, OneOf("b")},
		{15, OneOf("c")},
		{10, OneOf("d")},
	})
	wants := []frequency[string]{
		{0.50, "a"},
		{0.25, "b"},
		{0.15, "c"},
		{0.10, "d"},
	}
	testGenerator(t, gen, wants)
}

func TestStringsArePrintable(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 1000; i++ {
		for _, r := range String().Generate(rng) {
			if !unicode.IsPrint(r) {
				t.Errorf("Unprintable character %#U", r)
			}
		}
	}
}
