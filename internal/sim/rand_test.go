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
)

// newInts returns a new set of integers in the range [low, high).
// newInts panics if low >= high.
func newInts(low, high int) *ints {
	i := &ints{}
	i.reset(low, high)
	return i
}

func TestIntsHas(t *testing.T) {
	const low = 10
	const high = 100
	const n = high - low
	xs := newInts(low, high)
	in := map[int]bool{}  // elements in xs
	out := map[int]bool{} // elements not in xs
	for i := low; i < high; i++ {
		in[i] = true
	}

	rand := rand.New(rand.NewSource(time.Now().UnixMicro()))
	for i := 0; i < n; i++ {
		for x := range in {
			if !xs.has(x) {
				t.Errorf("%d missing", x)
			}
		}

		for x := range out {
			if xs.has(x) {
				t.Errorf("%d spuriously present", x)
			}
		}

		x := xs.pick(rand)
		xs.remove(x)
		delete(in, x)
		out[x] = true
	}
}

func TestIntsHasOutOfRange(t *testing.T) {
	xs := newInts(10, 20)
	for i := 0; i < 10; i++ {
		if xs.has(i) {
			t.Errorf("%d spuriously present", i)
		}
	}
	for i := 20; i < 30; i++ {
		if xs.has(i) {
			t.Errorf("%d spuriously present", i)
		}
	}
}

func TestIntsSize(t *testing.T) {
	const low = 10
	const high = 100
	const n = high - low
	xs := newInts(low, high)
	rand := rand.New(rand.NewSource(time.Now().UnixMicro()))
	for i := 0; i < n; i++ {
		if got, want := xs.size(), n-i; got != want {
			t.Errorf("size: got %d, want %d", got, want)
		}
		xs.remove(xs.pick(rand))
	}
	if xs.size() != 0 {
		t.Errorf("size: got %d, want 0", xs.size())
	}
}

func TestIntsPick(t *testing.T) {
	const low = 42
	const high = 9001
	xs := newInts(low, high)
	rand := rand.New(rand.NewSource(time.Now().UnixMicro()))
	for i := 0; i < 100; i++ {
		if x := xs.pick(rand); x < low || x >= high {
			t.Errorf("pick: %d not in range [%d, %d)", x, low, high)
		}
	}
}

func TestIntsDuplicateRemove(t *testing.T) {
	xs := newInts(10, 20)
	xs.remove(14)
	xs.remove(14)
	if xs.has(14) {
		t.Error("14 spuriously present")
	}
}

func TestWyRandDeterministic(t *testing.T) {
	for seed := uint64(0); seed < 100; seed++ {
		a := &wyrand{seed}
		b := &wyrand{seed}
		for i := 0; i < 100; i++ {
			w, x := a.Int63(), b.Int63()
			if w != x {
				t.Fatalf("seed %d Int63: %d != %d", seed, w, x)
			}
			y, z := a.Uint64(), b.Uint64()
			if y != z {
				t.Fatalf("seed %d Uint64: %d != %d", seed, y, z)
			}
		}
	}
}
