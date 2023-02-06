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

package main

import (
	"context"
	"fmt"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	weaver "github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/metrics"
)

var (
	reads   = metrics.NewCounter("chat_cache_reads", "Number of cache reads.")
	misses  = metrics.NewCounter("chat_cache_misses", "Number of cache misses.")
	writes  = metrics.NewCounter("chat_cache_writes", "Number of cache writes.")
	entries = metrics.NewGauge("chat_cache_entries", "Number of cache entries.")
	sizes   = metrics.NewHistogram(
		"chat_cache_value_sizes_bytes",
		"Histogram of sizes, in bytes, of cache values.",
		[]float64{10, 100, 1000, 10000, 100000, 1000000},
	)
)

const cacheSize = 65536

type LocalCache interface {
	Get(_ context.Context, key string) (string, error)
	Put(_ context.Context, key, val string) error
}

type localCache struct {
	weaver.Implements[LocalCache]
	mu    sync.Mutex
	cache *lru.Cache
	// TODO: Eviction policy.
}

func (c *localCache) Init(context.Context) error {
	cache, err := lru.New(cacheSize)
	c.cache = cache
	return err
}

// Get returns the cached value for key, or an error.
func (c *localCache) Get(_ context.Context, key string) (string, error) {
	reads.Add(1)
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.cache.Get(key)
	if !ok {
		misses.Add(1)
		return "", fmt.Errorf("key %q not found in cache", key)
	}
	return v.(string), nil
}

// Put stores key,val in the cache.
func (c *localCache) Put(_ context.Context, key, val string) error {
	writes.Add(1)
	sizes.Put(float64(len(val)))
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache.Add(key, val)
	entries.Set(float64(c.cache.Len()))
	return nil
}
