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

// Package versioned provides a linearizable generic register.
package versioned

import (
	"sync"

	"github.com/google/uuid"
)

// Versioned is a linearizable register storing a value of type T.  Each update
// to the value changes its unique (but not necessarily ordered) version.
// Spurious version changes are possible, i.e., the version may change even if
// the value hasn't.
//
// Like a sync.Mutex, Versioned should not be copied.
type Versioned[T any] struct {
	mu      sync.RWMutex
	changed sync.Cond
	Val     T
	version string
}

func Version[T any](val T) *Versioned[T] {
	v := &Versioned[T]{Val: val, version: uuid.New().String()}
	v.changed.L = &v.mu
	return v
}

// Lock acquires the write lock.
func (v *Versioned[T]) Lock() {
	v.mu.Lock()
}

// Unlock releases the write lock.
func (v *Versioned[T]) Unlock() {
	v.version = uuid.New().String()
	v.changed.Broadcast()
	v.mu.Unlock()
}

// RLock waits until the current version is different than the passed-in
// version, and then acquires the read lock and returns the new
// version.
func (v *Versioned[T]) RLock(version string) string {
	v.mu.RLock()
	if v.version != version {
		return v.version
	}

	// Upgrade the lock since we may wait on v.changed.
	v.mu.RUnlock()
	v.mu.Lock()
	for v.version == version {
		v.changed.Wait()
	}

	// Downgrade the lock.
	v.mu.Unlock()
	v.mu.RLock()
	return v.version
}

// RUnlock releases the read lock.
func (v *Versioned[T]) RUnlock() {
	v.mu.RUnlock()
}
