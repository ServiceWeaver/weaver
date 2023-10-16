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

// Package retry contains code to perform retries with exponential backoff.
//
// Example: loop until doSomething() returns true or context hits deadline or is canceled.
//
//	for r := retry.Begin(); r.Continue(ctx); {
//	  if doSomething() {
//	    break
//	  }
//	}
package retry

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"
)

// Retry holds state for managing retry loops with exponential backoff and jitter.
type Retry struct {
	options Options
	attempt int
}

// Options are the options that configure a retry loop. Before the ith
// iteration of a retry loop, retry.Continue() sleeps for a duration of
// BackoffMinDuration * BackoffMultiplier^i, with added jitter.
type Options struct {
	BackoffMultiplier  float64 // If specified, must be at least 1.
	BackoffMinDuration time.Duration
}

// DefaultOptions is the default set of Options.
var DefaultOptions = Options{
	BackoffMultiplier:  1.3,
	BackoffMinDuration: 10 * time.Millisecond,
}

var (
	rngMu sync.Mutex
	rng   *rand.Rand
)

// Begin initiates a new retry loop.
func Begin() *Retry {
	return BeginWithOptions(DefaultOptions)
}

// BeginWithOptions returns a new retry loop configured with the provided
// options.
//
// Example: Sleep 1 second, then 2 seconds, then 4 seconds, and so on.
//
//	opts := retry.Options{
//	  BackoffMultiplier: 2.0,
//	  BackoffMinDuration: time.Second,
//	}
//	for r := retry.BeginWithOptions(opts); r.Continue(ctx); {
//	  // Do nothing.
//	}
func BeginWithOptions(options Options) *Retry {
	return &Retry{options: options}
}

// Continue sleeps for an exponentially increasing interval (with jitter). It
// stops its sleep early and returns false if context becomes done. If the
// return value is false, ctx.Err() is guaranteed to be non-nil. The first
// call does not sleep.
func (r *Retry) Continue(ctx context.Context) bool {
	if r.attempt != 0 {
		randomized(ctx, backoffDelay(r.attempt, r.options))
	}
	r.attempt++
	return ctx.Err() == nil
}

// Reset resets a Retry to its initial state. Reset is useful if you want to
// retry an operation with exponential backoff, but only if it is failing. For
// example:
//
//	for r := retry.Begin(); r.Continue(ctx); {
//	    if err := doSomething(); err != nil {
//	        // Retry with backoff if we fail.
//	        continue
//	    }
//	    // Retry immediately if we succeed.
//	    r.Reset()
//	}
func (r *Retry) Reset() {
	r.attempt = 0
}

func backoffDelay(i int, opts Options) time.Duration {
	mult := math.Pow(opts.BackoffMultiplier, float64(i))
	return time.Duration(float64(opts.BackoffMinDuration) * mult)
}

// randomized sleeps for a random duration close to d, or until context is done,
// whichever occurs first.
func randomized(ctx context.Context, d time.Duration) {
	const jitter = 0.4
	mult := 1 - jitter*randomFloat() // Subtract up to 40%
	sleep(ctx, time.Duration(float64(d)*mult))
}

// sleep sleeps for the specified duration d, or until context is done,
// whichever occurs first.
func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return
	case <-t.C:
	}
}

func randomFloat() float64 {
	// Do not use the default rng since we do not want different processes
	// to pick the same deterministic random sequence.
	rngMu.Lock()
	defer rngMu.Unlock()
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return rng.Float64()
}
