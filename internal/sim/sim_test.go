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

package sim

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver"
)

// See TestSuccessfulSimulation.
type successfulWorkload struct {
	divmod weaver.Ref[divMod]
}

func (s *successfulWorkload) Init(r Registrar) error {
	r.RegisterGenerators("DivMod", intn{0, 100}, intn{1, 100})
	return nil
}

func (s *successfulWorkload) DivMod(ctx context.Context, x, y int) error {
	div, mod, err := s.divmod.Get().DivMod(ctx, x, y)
	if err != nil {
		// Ignore errors.
		return nil
	}
	if div != x/y {
		return fmt.Errorf("div %d/%d: got %d, want %d", x, y, div, x/y)
	}
	if mod != x%y {
		return fmt.Errorf("mod %d%%%d: got %d, want %d", x, y, mod, x%y)
	}
	return nil
}

func TestSuccessfulSimulation(t *testing.T) {
	// Run the simulator, ignoring any errors. The simulation should pass.
	opts := options{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 0.1,
		YieldRate:   0.5,
	}
	s := New(t, &successfulWorkload{}, Options{})
	results, err := s.runOne(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if results.Err != nil {
		t.Fatal(results.Err)
	}
}

// See TestUnsuccessfulSimulation.
type unsuccessfulWorkload struct {
	divmod weaver.Ref[divMod]
}

func (u *unsuccessfulWorkload) Init(r Registrar) error {
	r.RegisterGenerators("DivMod", intn{0, 100}, intn{0, 100})
	return nil
}

func (u *unsuccessfulWorkload) DivMod(ctx context.Context, x, y int) error {
	_, _, err := u.divmod.Get().DivMod(ctx, x, y)
	return err
}

func TestUnsuccessfulSimulation(t *testing.T) {
	// Run the simulator, erroring out if we ever have a zero denominator. The
	// simulation as a whole should fail with extremely high likelihood.
	opts := options{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 0.1,
		YieldRate:   0.5,
	}
	s := New(t, &unsuccessfulWorkload{}, Options{})
	results, err := s.runOne(context.Background(), opts)
	if err == nil && results.Err == nil {
		t.Fatal("unexpected success")
	}
}

func TestSimulateGraveyardEntries(t *testing.T) {
	// This test re-runs failed UnsuccessfulSimulation simulations.
	graveyard, err := readGraveyard(filepath.Join("testdata", "sim", "TestUnsuccessfulSimulation"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range graveyard {
		opts := options{
			Seed:        entry.Seed,
			NumReplicas: entry.NumReplicas,
			NumOps:      entry.NumOps,
			FailureRate: entry.FailureRate,
			YieldRate:   entry.YieldRate,
		}
		s := New(t, &unsuccessfulWorkload{}, Options{})
		results, err := s.runOne(context.Background(), opts)
		if err == nil && results.Err == nil {
			t.Fatal("unexpected success")
		}
	}
}

// See TestCancelledSimulation.
type cancellableWorkload struct {
	b weaver.Ref[blocker]
}

func (c *cancellableWorkload) Init(r Registrar) error {
	r.RegisterGenerators("Block")
	return nil
}

func (c *cancellableWorkload) Block(ctx context.Context) error {
	c.b.Get().Block(ctx)
	return ctx.Err()
}

func TestCancelledSimulation(t *testing.T) {
	// Run a blocking simulation and cancel it.
	opts := options{NumReplicas: 10, NumOps: 1000}
	s := New(t, &cancellableWorkload{}, Options{})

	const delay = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), delay)
	defer cancel()
	errs := make(chan error)
	go func() {
		results, err := s.runOne(ctx, opts)
		if err != nil {
			errs <- err
		} else {
			errs <- results.Err
		}
	}()

	failBefore := time.Now().Add(delay / 2)
	failAfter := time.After(delay * 2)
	select {
	case err := <-errs:
		if !errors.Is(err, ctx.Err()) {
			t.Fatalf("error: got %v, want %v", err, ctx.Err())
		}
		if time.Now().Before(failBefore) {
			t.Fatal("simulation cancelled prematurely")
		}
	case <-failAfter:
		t.Fatal("simulation not cancelled promptly")
	}
}

// See TestFailureRateZero.
type noFailureWorkload struct {
	divmod weaver.Ref[divMod]
}

func (n *noFailureWorkload) Init(r Registrar) error {
	r.RegisterGenerators("DivMod", intn{0, 100}, intn{1, 100})
	return nil
}

func (n *noFailureWorkload) DivMod(ctx context.Context, x, y int) error {
	_, _, err := n.divmod.Get().DivMod(ctx, x, y)
	return err
}

func TestFailureRateZero(t *testing.T) {
	// With FailureRate set to zero, no operation should fail.
	opts := options{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 0.0,
		YieldRate:   0.5,
	}
	s := New(t, &noFailureWorkload{}, Options{})
	results, err := s.runOne(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if results.Err != nil {
		t.Fatal(results.Err)
	}
}

// See TestFailureRateOne.
type totalFailureWorkload struct {
	divmod weaver.Ref[divMod]
}

func (t *totalFailureWorkload) Init(r Registrar) error {
	r.RegisterGenerators("DivMod", intn{0, 100}, intn{1, 100})
	return nil
}

func (t *totalFailureWorkload) DivMod(ctx context.Context, x, y int) error {
	_, _, err := t.divmod.Get().DivMod(ctx, x, y)
	if err == nil {
		return fmt.Errorf("unexpected success")
	}
	return nil
}

func TestFailureRateOne(t *testing.T) {
	// With FailureRate set to one, all operations should fail.
	opts := options{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 1.0,
		YieldRate:   0.5,
	}
	s := New(t, &totalFailureWorkload{}, Options{})
	results, err := s.runOne(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if results.Err != nil {
		t.Fatal(results.Err)
	}
}

// See TestInjectedErrors.
type injectedErrorWorkload struct {
	divmod weaver.Ref[divMod]

	mu       sync.Mutex
	nextId   int
	executed map[int]struct{}
	errored  map[int]struct{}
}

func (i *injectedErrorWorkload) Init(r Registrar) error {
	i.executed = map[int]struct{}{}
	i.errored = map[int]struct{}{}
	r.RegisterFake(Fake[divMod](injectedErrorDivMod{i}))
	r.RegisterGenerators("DivMod")
	return nil
}

func (i *injectedErrorWorkload) DivMod(ctx context.Context) error {
	i.mu.Lock()
	id := i.nextId
	i.nextId++
	i.mu.Unlock()

	_, _, err := i.divmod.Get().DivMod(ctx, id, id)
	if errors.Is(err, weaver.RemoteCallError) {
		i.mu.Lock()
		i.errored[id] = struct{}{}
		i.mu.Unlock()
	}

	// TODO(mwhittaker): Allow people to register a postcondition check with a
	// workload.
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.nextId == 1000 {
		failedWithoutExecuting := 0
		failedAfterExecuting := 0
		for id := range i.errored {
			if _, ok := i.executed[id]; ok {
				failedAfterExecuting++
			} else {
				failedWithoutExecuting++
			}
		}
		if failedWithoutExecuting == 0 {
			return fmt.Errorf("no calls failed without executing")
		}
		if failedAfterExecuting == 0 {
			return fmt.Errorf("no calls failed after executing")
		}
	}
	return nil
}

type injectedErrorDivMod struct {
	i *injectedErrorWorkload
}

func (i injectedErrorDivMod) DivMod(_ context.Context, x, _ int) (int, int, error) {
	i.i.mu.Lock()
	defer i.i.mu.Unlock()
	i.i.executed[x] = struct{}{}
	return 0, 0, nil
}

func TestInjectedErrors(t *testing.T) {
	// An injected RemoteCallError can happen before or after a call is
	// executed. Record the calls that error out without executing and the
	// calls that error out after executing. Both should happen.
	opts := options{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 0.5,
		YieldRate:   0.5,
	}
	s := New(t, &injectedErrorWorkload{}, Options{})
	results, err := s.runOne(context.Background(), opts)
	if err != nil || results.Err != nil {
		t.Fatal(err)
	}
}

// See TestFakes.
type fakeDivMod struct{}

func (fakeDivMod) DivMod(context.Context, int, int) (int, int, error) {
	return 42, 42, nil
}

type fakeWorkload struct {
	divmod weaver.Ref[divMod]
}

func (f *fakeWorkload) Init(r Registrar) error {
	r.RegisterFake(Fake[divMod](fakeDivMod{}))
	r.RegisterGenerators("DivMod", intn{0, 100}, intn{1, 100})
	return nil
}

func (f *fakeWorkload) DivMod(ctx context.Context, x, y int) error {
	div, mod, err := f.divmod.Get().DivMod(ctx, x, y)
	if err != nil {
		// Ignore errors.
		return nil
	}
	if div != 42 {
		return fmt.Errorf("div %d/%d: got %d, want %d", x, y, div, 42)
	}
	if mod != 42 {
		return fmt.Errorf("mod %d%%%d: got %d, want %d", x, y, mod, 42)
	}
	return nil
}

func TestFakes(t *testing.T) {
	opts := options{
		NumReplicas: 10,
		NumOps:      1000,
		YieldRate:   0.5,
	}
	s := New(t, &fakeWorkload{}, Options{})
	results, err := s.runOne(context.Background(), opts)
	if err != nil || results.Err != nil {
		t.Fatal(err)
	}
}

func TestExtractIDs(t *testing.T) {
	const traceID = 42
	const spanID = 9001
	ctx := withIDs(context.Background(), traceID, spanID)
	if got, _ := extractIDs(ctx); got != traceID {
		t.Errorf("trace id: got %d, want %d", got, traceID)
	}
	if _, got := extractIDs(ctx); got != spanID {
		t.Errorf("span id: got %d, want %d", got, spanID)
	}
}

func TestExtractIDsOnInvalidContext(t *testing.T) {
	traceID, spanID := extractIDs(context.Background())
	if traceID != 0 {
		t.Errorf("trace id: got %d, want 0", traceID)
	}
	if spanID != 0 {
		t.Errorf("span id: got %d, want 0", spanID)
	}
}

// A workload with no method calls and no generators.
type noCallsNoGenWorkload struct{}

func (*noCallsNoGenWorkload) Init(r Registrar) error {
	r.RegisterGenerators("Foo")
	return nil
}

func (*noCallsNoGenWorkload) Foo(context.Context) error {
	return nil
}

// A workload with no method calls.
type noCallsWorkload struct{}

func (*noCallsWorkload) Init(r Registrar) error {
	r.RegisterGenerators("Foo", integers{})
	return nil
}

func (*noCallsWorkload) Foo(context.Context, int) error {
	return nil
}

// A workload with one method call per op.
type oneCallWorkload struct {
	id weaver.Ref[identity]
}

func (*oneCallWorkload) Init(r Registrar) error {
	r.RegisterGenerators("Foo", integers{})
	return nil
}

func (o *oneCallWorkload) Foo(ctx context.Context, x int) error {
	o.id.Get().Identity(ctx, x)
	return nil
}

func BenchmarkWorkloads(b *testing.B) {
	for _, bench := range []struct {
		name     string
		workload Workload
	}{
		{"NoCallsNoGen", &noCallsNoGenWorkload{}},
		{"NoCalls", &noCallsWorkload{}},
		{"OneCall", &oneCallWorkload{}},
	} {
		b.Run(bench.name, func(b *testing.B) {
			b.ReportAllocs()
			s := New(b, bench.workload, Options{})
			opts := options{
				NumReplicas: 1,
				NumOps:      1000,
				FailureRate: 0,
				YieldRate:   1,
			}
			for i := 0; i < b.N; i++ {
				results, err := s.runOne(context.Background(), opts)
				if err != nil {
					b.Fatal(err)
				}
				if results.Err != nil {
					b.Fatal(results.Err)
				}
			}
		})
	}
}
