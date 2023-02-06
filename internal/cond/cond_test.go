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

package cond_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/ServiceWeaver/weaver/internal/cond"
)

// TestCancelledWait tests that a Wait on a cancelled context returns promptly
// with the context's error.
func TestCancelledWait(t *testing.T) {
	var m sync.Mutex
	c := cond.NewCond(&m)
	errs := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		m.Lock()
		defer m.Unlock()
		errs <- c.Wait(ctx)
	}()

	timer := time.NewTimer(100 * time.Millisecond)
	cancel()
	select {
	case err := <-errs:
		if got, want := err, ctx.Err(); got != want {
			t.Fatalf("bad error: got %v, want %v", got, want)
		}
	case <-timer.C:
		t.Fatal("cancelled Wait() did not terminate promptly")
	}
}

// TestSignal tests that a Signal wakes up a goroutine blocked on Wait.
func TestSignal(t *testing.T) {
	var m sync.Mutex
	c := cond.NewCond(&m)
	errs := make(chan error, 1)
	ctx := context.Background()

	waiting := false
	waiter := func() {
		m.Lock()
		defer m.Unlock()
		waiting = true
		errs <- c.Wait(ctx)
	}
	signaler := func() {
		// If we call Signal before the waiter goroutine calls Wait, then the
		// Signal is a noop and the Wait blocks forever.
		for {
			m.Lock()
			b := waiting
			m.Unlock()

			if b {
				c.Signal()
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
	}
	go waiter()
	go signaler()

	timer := time.NewTimer(100 * time.Millisecond)
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("unexpected error %v", err)
		}
	case <-timer.C:
		t.Fatal("signalled Wait() did not terminate promptly")
	}
}

// TestBroadcast tests that a Broadcast wakes up all goroutines blocked on
// Wait.
func TestBroadcast(t *testing.T) {
	var m sync.Mutex
	c := cond.NewCond(&m)
	numWaiters := 10
	errs := make(chan error, numWaiters)
	ctx := context.Background()

	numWaiting := 0
	waiter := func() {
		m.Lock()
		defer m.Unlock()
		numWaiting += 1
		errs <- c.Wait(ctx)
	}
	broadcaster := func() {
		// If we call Broadcast before the waiter goroutine calls Wait, then
		// the Broadcast is a noop and the Wait blocks forever.
		for {
			m.Lock()
			b := numWaiting == numWaiters
			m.Unlock()

			if b {
				c.Broadcast()
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
	}
	for i := 0; i < numWaiters; i++ {
		go waiter()
	}
	go broadcaster()

	timer := time.NewTimer(100 * time.Millisecond)
	for i := 0; i < numWaiters; i++ {
		select {
		case err := <-errs:
			if err != nil {
				t.Errorf("unexpected error %v", err)
			}
		case <-timer.C:
			t.Fatal("broadcasted Wait()'s did not terminate promptly")
		}
	}
}

// queue is a blocking concurrent queue.
type queue interface {
	Pop(ctx context.Context) (int, error)
	Push(x int) error
}

// broadcaster is a queue that uses cond.Broadcast.
type broadcaster struct {
	lockedBroadcast bool // if true, hold m when broadcasting

	m        sync.Mutex // used by nonempty, guards xs
	nonempty cond.Cond  // triggered when xs is non-empty
	xs       []int      // the underlying queue elements
}

// Check that broadcaster implements the queue interface.
var _ queue = &broadcaster{}

func newBroadcaster(lockedBroadcast bool) *broadcaster {
	var q broadcaster
	q.nonempty.L = &q.m
	q.xs = []int{}
	q.lockedBroadcast = lockedBroadcast
	return &q
}

func (q *broadcaster) Pop(ctx context.Context) (int, error) {
	q.m.Lock()
	defer q.m.Unlock()

	for len(q.xs) == 0 {
		if err := q.nonempty.Wait(ctx); err != nil {
			return 0, err
		}
	}
	x := q.xs[0]
	q.xs = q.xs[1:]
	return x, nil
}

func (q *broadcaster) Push(x int) error {
	q.m.Lock()
	if q.lockedBroadcast {
		defer q.m.Unlock()
	}
	q.xs = append(q.xs, x)
	if !q.lockedBroadcast {
		q.m.Unlock()
	}
	q.nonempty.Broadcast()
	return nil
}

// signaler is a queue that uses cond.Signal.
type signaler struct {
	lockedSignal bool // if true, hold m when signalling

	m        sync.Mutex // used by nonempty, guards xs
	nonempty cond.Cond  // signalled when xs is non-empty
	xs       []int      // the underlying queue elements
}

// Check that signaler implements the queue interface.
var _ queue = &signaler{}

func newSignaler(lockedSignal bool) *signaler {
	var q signaler
	q.nonempty.L = &q.m
	q.xs = []int{}
	q.lockedSignal = lockedSignal
	return &q
}

func (q *signaler) Pop(ctx context.Context) (int, error) {
	q.m.Lock()
	if q.lockedSignal {
		defer q.m.Unlock()
	}

	for len(q.xs) == 0 {
		if err := q.nonempty.Wait(ctx); err != nil {
			if !q.lockedSignal {
				q.m.Unlock()
			}
			return 0, err
		}
	}
	x := q.xs[0]
	q.xs = q.xs[1:]
	n := len(q.xs)

	if !q.lockedSignal {
		q.m.Unlock()
	}
	if n > 0 {
		q.nonempty.Signal()
	}
	return x, nil
}

func (q *signaler) Push(x int) error {
	q.m.Lock()
	if q.lockedSignal {
		defer q.m.Unlock()
	}
	q.xs = append(q.xs, x)
	if !q.lockedSignal {
		q.m.Unlock()
	}
	q.nonempty.Signal()
	return nil
}

func TestQueues(t *testing.T) {
	for _, q := range []struct {
		name string
		q    func() queue
	}{
		{"LockedBroadcaster", func() queue { return newBroadcaster(true) }},
		{"UnlockedBroadcaster", func() queue { return newBroadcaster(false) }},
		{"LockedSignaler", func() queue { return newSignaler(true) }},
		{"UnlockedSignaler", func() queue { return newSignaler(false) }},
	} {
		for _, test := range []struct {
			name string
			f    func(*testing.T, queue)
		}{
			{"TestPopEmpty", testPopEmpty},
			{"TestPushPops", testPushPops},
			{"TestPushNoPops", testPushNoPops},
		} {
			name := fmt.Sprintf("%s/%s", q.name, test.name)
			t.Run(name, func(t *testing.T) { test.f(t, q.q()) })
		}
	}
}

// testPopEmpty tests that Pop respects the context it receives.
func testPopEmpty(t *testing.T, q queue) {
	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, 1)
	go func() {
		_, err := q.Pop(ctx)
		errs <- err
	}()

	timer := time.NewTimer(100 * time.Millisecond)
	cancel()
	select {
	case err := <-errs:
		if got, want := err, ctx.Err(); got != want {
			t.Fatalf("bad error: got %v, want %v", got, want)
		}
	case <-timer.C:
		t.Fatal("cancelled Pop() did not terminate promptly")
	}
}

// testPushPops tests that concurrent pushes and pops work correctly.
func testPushPops(t *testing.T, q queue) {
	numPoppers := 3
	numPushers := 10
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	popped := make(chan int, numPushers)
	popper := func() error {
		for {
			x, err := q.Pop(ctx)
			if err != nil {
				return err
			}
			popped <- x
		}
	}

	pusher := func(i int) error {
		return q.Push(i)
	}

	// Launch goroutines.
	var done sync.WaitGroup
	done.Add(numPoppers + numPushers)
	errs := make(chan error, numPoppers+numPushers)
	for i := 0; i < numPoppers; i++ {
		go func() {
			defer done.Done()
			errs <- popper()
		}()
	}
	for i := 0; i < numPushers; i++ {
		i := i
		go func() {
			defer done.Done()
			errs <- pusher(i)
		}()
	}
	done.Wait()

	// Check for errors.
	close(errs)
	for err := range errs {
		if err != nil && err != ctx.Err() {
			t.Error(err)
		}
	}

	// Check popped values.
	close(popped)
	got := []int{}
	for x := range popped {
		got = append(got, x)
	}
	want := []int{}
	for i := 0; i < numPushers; i++ {
		want = append(want, i)
	}
	less := func(x, y int) bool { return x < y }
	if diff := cmp.Diff(want, got, cmpopts.SortSlices(less)); diff != "" {
		t.Fatalf("(-want +got):\n%s", diff)
	}
}

// testPushNoPops tests that Push works correctly when there are no poppers. In
// turn, this tests that a Signal or Brodacast works correctly when there are
// no waiters.
func testPushNoPops(t *testing.T, q queue) {
	for i := 0; i < 10; i++ {
		if err := q.Push(i); err != nil {
			t.Fatal(err)
		}
	}
}
