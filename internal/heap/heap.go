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

// Package heap provide a min-heap implementation called Heap.
package heap

import "container/heap"

// Heap is a generic min-heap. Modifying an element while it is on the heap
// invalidates the heap.
type Heap[T any] struct {
	// Heap wraps the heap package in the standard library, making it more
	// ergonomic. For example, heap.Pop can panic when called on an empty heap,
	// whereas Heap.Pop returns a false ok value when called on an empty heap.
	// Conversely, Heap is slower than the heap package in the standard
	// library, so prefer the standard library package if you need good
	// performance.
	h *sliceheap[T]
}

// New returns a new empty heap, with elements sorted using the provided
// comparator function.
func New[T any](less func(x, y T) bool) *Heap[T] {
	h := &sliceheap[T]{less: less}
	heap.Init(h)
	return &Heap[T]{h: h}
}

// Len returns the length of the heap.
func (h *Heap[T]) Len() int {
	return h.h.Len()
}

// Push pushes an element onto the heap.
func (h *Heap[T]) Push(val T) {
	heap.Push(h.h, val)
}

// Peek returns the least element from the heap, if the heap is non-empty.
// Unlike Pop, Peek does not modify the heap.
func (h *Heap[T]) Peek() (val T, ok bool) {
	if h.h.Len() == 0 {
		return val, false
	}
	return h.h.xs[0], true
}

// Pop pops the least element from the heap, if the heap is non-empty.
func (h *Heap[T]) Pop() (val T, ok bool) {
	if h.h.Len() == 0 {
		return val, false
	}
	return heap.Pop(h.h).(T), true
}

// sliceheap is an array-backed heap that implements the heap.Interface
// interface, allowing us to call heap operations on it.
type sliceheap[T any] struct {
	less func(x, y T) bool // orders xs
	xs   []T               // the heap
}

// Len implements the heap.Interface interface.
func (h *sliceheap[T]) Len() int {
	return len(h.xs)
}

// Less implements the heap.Interface interface.
func (h *sliceheap[T]) Less(i, j int) bool {
	return h.less(h.xs[i], h.xs[j])
}

// Swap implements the heap.Interface interface.
func (h *sliceheap[T]) Swap(i, j int) {
	h.xs[i], h.xs[j] = h.xs[j], h.xs[i]
}

// Push implements the heap.Interface interface.
func (h *sliceheap[T]) Push(x interface{}) {
	h.xs = append(h.xs, x.(T))
}

// Pop implements the heap.Interface interface.
func (h *sliceheap[T]) Pop() interface{} {
	x := h.xs[len(h.xs)-1]
	h.xs = h.xs[:len(h.xs)-1]
	return x
}
