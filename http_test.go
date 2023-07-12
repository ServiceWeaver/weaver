// Copyright 2022 Google LLC
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

package weaver

import (
	"math"
	"math/rand"
	"net/http"
	"testing"
	"time"
)

func ExampleInstrumentHandler() {
	var mux http.ServeMux
	mux.Handle("/foo", InstrumentHandler("foo", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { /*...*/ })))
	mux.Handle("/bar", InstrumentHandler("bar", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { /*...*/ })))
	http.ListenAndServe(":9000", &mux)
}

type fixedRandSource int64

var _ rand.Source = fixedRandSource(0)

func (s fixedRandSource) Int63() int64 { return int64(s) }
func (s fixedRandSource) Seed(int64)   {}

func TestTraceSampler(t *testing.T) {
	// We run with an interval of a 1s and an "rng" that always returns 0.5, so
	// each sampling gap should be 1s.
	rng := rand.New(fixedRandSource(math.MaxInt64 / 2))
	s := newTraceSampler(time.Second, rng)
	now := time.Now()
	const numTimeIntervals = 100
	var numTraces int
	for i := 0; i < numTimeIntervals; i++ {
		now = now.Add(time.Second)
		for j := 0; j < 10; j++ {
			if s.shouldTrace(now.Add(time.Millisecond)) {
				numTraces++
			}
		}
	}
	if numTraces != numTimeIntervals {
		t.Fatalf("unexpected number of traces sampled: want %d, got %d", numTimeIntervals, numTraces)
	}

}
