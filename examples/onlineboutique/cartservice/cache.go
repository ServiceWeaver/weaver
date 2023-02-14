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

package cartservice

import (
	"context"

	lru "github.com/hashicorp/golang-lru"
	"github.com/ServiceWeaver/weaver"
)

const cacheSize = 1 << 20 // 1M entries

type errNotFound struct{}

var _ error = errNotFound{}

func (e errNotFound) Error() string { return "not found" }

type cache[K, V any] struct {
	lru *lru.Cache
}

func newCache[K, V any]() (*cache[K, V], error) {
	l, err := lru.New(cacheSize)
	if err != nil {
		return nil, err
	}
	return &cache[K, V]{
		lru: l,
	}, nil
}

// Add adds the given (key, val) pair to the cache.
func (c *cache[K, V]) Add(_ context.Context, key K, val V) error {
	c.lru.Add(key, val)
	return nil
}

// Get returns the value associated with the given key in the cache, or
// ErrNotFound if there is no associated value.
func (c *cache[K, V]) Get(_ context.Context, key K) (V, error) {
	val, ok := c.lru.Get(key)
	if !ok {
		var v V
		return v, errNotFound{}
	}
	return val.(V), nil
}

// Remove removes an entry with the given key from the cache.
func (c *cache[K, V]) Remove(_ context.Context, key K) (bool, error) {
	return c.lru.Remove(key), nil
}

// TODO(spetrovic): Allow the cache struct to reside in a different package.

type cartCache interface {
	Add(context.Context, string, []CartItem) error
	Get(context.Context, string) ([]CartItem, error)
	Remove(context.Context, string) (bool, error)
}

type cartCacheImpl struct {
	weaver.Implements[cartCache]
	weaver.WithRouter[cartCacheRouter]

	*cache[string, []CartItem]
}

func (c *cartCacheImpl) Init(context.Context) error {
	cache, err := newCache[string, []CartItem]()
	c.cache = cache
	return err
}

type cartCacheRouter struct{}

func (cartCacheRouter) Add(_ context.Context, key string, value []CartItem) string { return key }
func (cartCacheRouter) Get(_ context.Context, key string) string                   { return key }
func (cartCacheRouter) Remove(_ context.Context, key string) string                { return key }
