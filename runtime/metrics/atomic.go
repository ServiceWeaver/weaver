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
	"sync/atomic"
)

// atomicFloat64 provides atomic storage for float64.
type atomicFloat64 struct {
	v atomic.Uint64 // stores result of math.Float64bits
}

// get returns the current value stored in f.
func (f *atomicFloat64) get() float64 { return math.Float64frombits(f.v.Load()) }

// set stores v in f.
func (f *atomicFloat64) set(v float64) { f.v.Store(math.Float64bits(v)) }

// add atomically adds v to f.
func (f *atomicFloat64) add(v float64) {
	// Use compare-and-swap to change the stored representation
	// atomically from old value to old value + v.
	for {
		cur := f.v.Load()
		next := math.Float64bits(math.Float64frombits(cur) + v)
		if f.v.CompareAndSwap(cur, next) {
			return
		}
	}
}
