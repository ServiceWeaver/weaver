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

package metrics

import (
	"math"
	"sync"
	"testing"
)

var testFloats = []float64{math.Inf(-1), -2, -1, 0.5, -0, 0, 0.5, 1, 2, math.Inf(1)}

func TestAtomicFloat64GetSet(t *testing.T) {
	var f atomicFloat64
	for _, v := range testFloats {
		f.set(v)
		if got := f.get(); got != v {
			t.Errorf("get(set(%v)) = %v", v, got)
		}
	}
}

func TestAtomicFloat64Add(t *testing.T) {
	for _, a := range testFloats {
		for _, b := range testFloats {
			var v atomicFloat64
			v.add(a)
			v.add(b)
			want := a + b
			got := v.get()

			same := (math.IsNaN(want) && math.IsNaN(got)) || (want == got)
			if !same {
				t.Errorf("Add(%v) to %v produced %v, expecting %v", b, a, got, want)
			}
		}
	}
}

func TestAtomicFloat64Parallelism(t *testing.T) {
	var f atomicFloat64

	// Add to f from multiple goroutines in parallel.
	const iters = 1000000
	const parallelism = 8
	var wg sync.WaitGroup
	wg.Add(parallelism)
	for i := 0; i < parallelism; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				f.add(1.0)
			}
		}()
	}
	wg.Wait()

	want := float64(iters * parallelism)
	got := f.get()
	if got != want {
		t.Errorf("concurrent adds produced %v, expecting %v", got, want)
	}
}
