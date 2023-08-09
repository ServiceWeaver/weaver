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

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ServiceWeaver/weaver"
	lru "github.com/hashicorp/golang-lru/v2"
)

// The size of a factorer's LRU cache.
const cacheSize = 100

type Factorer interface {
	// Factors returns the factors of the provided non-positive integer in
	// ascending order. Note that Factors returns *all* factors, not just the
	// prime factors.
	Factors(context.Context, int) ([]int, error)
}

type factorer struct {
	weaver.Implements[Factorer] // factorer implements the Factorer component
	weaver.WithRouter[router]   // factorer's methods are routed by router

	cache *lru.Cache[int, []int] // maps integers to their factors
}

func (f *factorer) Init(_ context.Context) error {
	cache, err := lru.New[int, []int](cacheSize)
	f.cache = cache
	return err
}

func (f *factorer) Factors(ctx context.Context, x int) ([]int, error) {
	f.Logger(ctx).Debug("Factor", "pid", os.Getpid(), "x", x)

	// Sanity check arguments.
	if x <= 0 {
		return nil, fmt.Errorf("non-positive x: %d", x)
	}

	// Try to fetch the factors from the cache.
	factors, ok := f.cache.Get(x)
	if ok {
		return factors, nil
	}

	// Compute the factors from scratch, and cache them. Since this code is
	// just meant to illustrate Service Weaver features, we use a simplistic
	// approach to finding factors.
	for i := 1; i <= x; i++ {
		if x%i == 0 {
			factors = append(factors, i)
		}
	}
	f.cache.Add(x, factors)
	return factors, nil
}

// router routes factorer's method calls.
type router struct{}

// Factors routes calls to factorer.Factors.
func (r *router) Factors(_ context.Context, x int) int {
	// Factors returns a routing key, here x. Calls to factorer.Factors with
	// the same routing key will tend to get routed to the same component
	// instance.
	return x
}
