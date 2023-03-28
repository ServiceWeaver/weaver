// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package queue_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/internal/queue"
	"golang.org/x/sync/errgroup"
)

const x = 42

func pop[E any](q *queue.Queue[E]) E {
	elem, err := q.Pop(context.Background())
	if err != nil {
		panic(err)
	}
	return elem

}

func TestPushThenPop(t *testing.T) {
	var q queue.Queue[int]
	q.Push(x)
	if got, want := pop(&q), x; got != want {
		t.Fatalf("Pop: got %v, want %v", got, want)
	}
}

func TestPopThenPush(t *testing.T) {
	var q queue.Queue[int]
	go q.Push(x)
	if got, want := pop(&q), x; got != want {
		t.Fatalf("Pop: got %v, want %v", got, want)
	}
}

func TestMultiplePoprs(t *testing.T) {
	var q queue.Queue[int]
	var group errgroup.Group
	var sum atomic.Int64
	for i := 0; i < 10; i++ {
		group.Go(func() error {
			got, err := q.Pop(context.Background())
			if err != nil {
				return err
			}
			sum.Add(int64(got))
			return nil
		})
	}
	time.Sleep(20 * time.Millisecond) // Give poprs a chance to block.
	for i := 1; i < 11; i++ {
		q.Push(i)
	}
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
	if got, want := sum.Load(), int64(55); got != want {
		t.Fatalf("Pop: got %v, want %v", got, want)
	}
}

func TestMultiplePushrs(t *testing.T) {
	var q queue.Queue[int]
	for i := 1; i < 11; i++ {
		i := i
		go q.Push(i)
	}
	var sum int
	for i := 0; i < 10; i++ {
		sum += pop(&q)
	}
	if sum != 55 {
		t.Fatalf("Pop: got %v, want 55", sum)
	}
}

func TestContextCancel(t *testing.T) {
	var q queue.Queue[int]
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond) // Give popr a chance to block
		cancel()
	}()
	if _, err := q.Pop(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Pop: got %v, want %v", err, context.Canceled)
	}
}
