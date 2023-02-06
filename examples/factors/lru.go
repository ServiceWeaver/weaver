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

import lru "github.com/hashicorp/golang-lru"

// LRU[K, V] is a thread-safe LRU cache with keys and values of type K and V.
type LRU[K, V any] struct {
	cache *lru.Cache
}

// NewLRU returns a new LRU cache with the provided size.
func NewLRU[K, V any](size int) (*LRU[K, V], error) {
	cache, err := lru.New(size)
	return &LRU[K, V]{cache}, err
}

// Get returns the value associated with the provided key. Getting a value also
// "uses" it in the LRU sense.
func (l *LRU[K, V]) Get(key K) (value V, ok bool) {
	v, ok := l.cache.Get(key)
	if !ok {
		var zero V
		return zero, false
	}
	return v.(V), ok
}

// Put stores the provided key, value pair in the LRU, evicting the least
// recently used item if the cache size is exceeded. Putting a value also
// "uses" it in the LRU sense.
func (l *LRU[K, V]) Put(key K, value V) {
	l.cache.Add(key, value)
}

// Len returns the number of entries currently stored in the LRU.
func (l *LRU[K, V]) Len() int {
	return l.cache.Len()
}
