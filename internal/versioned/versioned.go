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

package versioned

import (
	"sync"

	"github.com/google/uuid"
)

// Versioned[T] is a linearizable register storing an element of type T. Every
// time the value changes, it is assigned a unique (but not necessarily
// ordered) version.
//
// A Versioned[T] is thread-safe. Like a sync.Mutex, it should not be copied.
type Versioned[T any] struct {
	mu      sync.Mutex
	updated sync.Cond
	val     T
	version string
}

// Version returns a new Versioned with the provided value.
func Version[T any](val T) *Versioned[T] {
	v := &Versioned[T]{
		val:     val,
		version: uuid.New().String(),
	}
	v.updated.L = &v.mu
	return v
}

// Get returns the value stored in the register, along with its version. If the
// provided version is empty, the latest value is returned. If the provided
// version is not empty, then Get blocks until a version newer than the one
// provided is present and then returns it.
func (v *Versioned[T]) Get(version string) (T, string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for v.version == version {
		v.updated.Wait()
	}
	return v.val, v.version
}

// Set sets the value of the register and returns the new value's version.
func (v *Versioned[T]) Set(val T) string {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.val = val
	v.version = uuid.New().String()
	v.updated.Broadcast()
	return v.version
}
