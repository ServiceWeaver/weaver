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

package heap_test

import (
	stdheap "container/heap"
	"fmt"
	"strconv"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/heap"
)

func Example_heapSort() {
	less := func(x, y int) bool { return x < y }
	ints := heap.New(less)
	unsorted := []int{2, 9, 6, 3, 8, 7, 4, 1, 5}
	sorted := []int{}

	for _, x := range unsorted {
		ints.Push(x)
	}
	for ints.Len() > 0 {
		x, _ := ints.Pop()
		sorted = append(sorted, x)
	}
	fmt.Println(sorted)
	// Output: [1 2 3 4 5 6 7 8 9]
}

func ExampleNew() {
	// Create a heap of ints.
	heap.New(func(x, y int) bool { return x < y })

	// Create a heap of strings.
	heap.New(func(x, y string) bool { return x < y })
}

func ExampleHeap_Peek() {
	ints := heap.New(func(x, y int) bool { return x < y })
	fmt.Println(ints.Peek())
	ints.Push(42)
	fmt.Println(ints.Peek())
	// Output:
	// 0 false
	// 42 true
}

func ExampleHeap_Pop() {
	ints := heap.New(func(x, y int) bool { return x < y })
	ints.Push(2)
	ints.Push(1)
	fmt.Println(ints.Pop())
	fmt.Println(ints.Pop())
	fmt.Println(ints.Pop())
	// Output:
	// 1 true
	// 2 true
	// 0 false
}

func BenchmarkHeap(b *testing.B) {
	for _, size := range []int{100, 1000, 10000, 100000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				less := func(x, y int) bool { return x < y }
				h := heap.New(less)
				for i := size; i >= 0; i-- {
					h.Push(i)
				}
				for i := 0; i < size; i++ {
					h.Pop()
				}
			}
		})
	}
}

func BenchmarkStdHeap(b *testing.B) {
	for _, size := range []int{100, 1000, 10000, 100000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				h := &intHeap{}
				stdheap.Init(h)
				for i := size; i >= 0; i-- {
					stdheap.Push(h, i)
				}
				for i := 0; i < size; i++ {
					stdheap.Pop(h)
				}
			}
		})
	}
}

// This implementation is taken from directly from [1].
//
// [1]: https://pkg.go.dev/container/heap#example-package-IntHeap
type intHeap []int

func (h intHeap) Len() int           { return len(h) }
func (h intHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h intHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *intHeap) Push(x interface{}) {
	*h = append(*h, x.(int))
}

func (h *intHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
