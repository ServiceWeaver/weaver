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

	"github.com/ServiceWeaver/weaver"
	lru "github.com/hashicorp/golang-lru/v2"
)

const cacheSize = 1 << 20 // 1M entries

type errNotFound struct{}

var _ error = errNotFound{}

func (e errNotFound) Error() string { return "not found" }

// TODO(spetrovic): Allow the cache struct to reside in a different package.

type cartCache interface {
	Add(context.Context, string, []CartItem) error
	Get(context.Context, string) ([]CartItem, error)
	Remove(context.Context, string) (bool, error)
}

type cartCacheImpl struct {
	weaver.Implements[cartCache]
	weaver.WithRouter[cartCacheRouter]

	cache *lru.Cache[string, []CartItem]
}

func (c *cartCacheImpl) Init(context.Context) error {
	cache, err := lru.New[string, []CartItem](cacheSize)
	c.cache = cache
	return err
}

// Add adds the given (key, val) pair to the cache.
func (c *cartCacheImpl) Add(_ context.Context, key string, val []CartItem) error {
	c.cache.Add(key, val)
	return nil
}

// Get returns the value associated with the given key in the cache, or
// ErrNotFound if there is no associated value.
func (c *cartCacheImpl) Get(_ context.Context, key string) ([]CartItem, error) {
	val, ok := c.cache.Get(key)
	if !ok {
		return nil, errNotFound{}
	}
	return val, nil
}

// Remove removes an entry with the given key from the cache.
func (c *cartCacheImpl) Remove(_ context.Context, key string) (bool, error) {
	return c.cache.Remove(key), nil
}

type cartCacheRouter struct{}

func (cartCacheRouter) Add(_ context.Context, key string, value []CartItem) string { return key }
func (cartCacheRouter) Get(_ context.Context, key string) string                   { return key }
func (cartCacheRouter) Remove(_ context.Context, key string) string                { return key }
