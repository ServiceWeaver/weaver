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

// Package register implements a write-once register.
package register

import (
	"fmt"
	"sync"
)

// WriteOnce is a concurrent write-once register.
type WriteOnce[T any] struct {
	mu      sync.Mutex
	c       sync.Cond
	written bool
	val     T
}

// Write writes to the register, or panics if the register was already written.
func (w *WriteOnce[T]) Write(val T) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.init()

	if w.written {
		panic(fmt.Sprintf("WriteOnce written more than once: old %v, new %v", w.val, val))
	}
	w.val = val
	w.written = true
	w.c.Broadcast()
}

// TryWrite tries to write to the register and returns if the write succeeds.
func (w *WriteOnce[T]) TryWrite(val T) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.init()

	if w.written {
		return false
	}
	w.val = val
	w.written = true
	w.c.Broadcast()
	return true
}

// Read returns the value of the register, blocking until it is written.
func (w *WriteOnce[T]) Read() T {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.init()

	for !w.written {
		w.c.Wait()
	}
	return w.val
}

// init initializes the register. We have an init method rather than a
// WriteOnce constructor so that the zero value of WriteOnce is valid.
//
// REQUIRES: w.mu is held.
func (w *WriteOnce[T]) init() {
	if w.c.L == nil {
		w.c.L = &w.mu
	}
}
