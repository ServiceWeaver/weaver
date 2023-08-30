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
	"math/rand"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/reflection"
)

type pair struct {
	x, y int
}

func simulator(t *testing.T, opts Options) *Simulator {
	t.Helper()
	sim, err := New(t.Name(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return sim
}

func TestSuccessfulSimulation(t *testing.T) {
	// Run the simulator, ignoring any errors. The simulation as a whole should
	// pass.
	opts := Options{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 0.1,
		YieldRate:   0.5,
	}
	sim := simulator(t, opts)
	RegisterOp(sim, Op[pair]{
		Name: "divmod",
		Gen: func(r *rand.Rand) pair {
			return pair{1 + rand.Intn(100), 1 + rand.Intn(100)}
		},
		Func: func(ctx context.Context, p pair, dm divMod) error {
			div, mod, err := dm.DivMod(ctx, p.x, p.y)
			if err != nil {
				// Swallow errors.
				return nil
			}
			if div != p.x/p.y {
				return fmt.Errorf("div %d/%d: got %d, want %d", p.x, p.y, div, p.x/p.y)
			}
			if mod != p.x%p.y {
				return fmt.Errorf("mod %d%%%d: got %d, want %d", p.x, p.y, mod, p.x%p.y)
			}
			return nil
		},
	})
	results, err := sim.Simulate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if results.Err != nil {
		t.Fatal(results.Err)
	}
}

func TestUnsuccessfulSimulation(t *testing.T) {
	// Run the simulator, erroring out if we ever have a zero denominator. The
	// simulation as a whole should fail with extremely high likelihood.
	opts := Options{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 0.1,
		YieldRate:   0.5,
	}
	sim := simulator(t, opts)
	RegisterOp(sim, Op[pair]{
		Name: "divmod",
		Gen: func(r *rand.Rand) pair {
			return pair{rand.Intn(100), rand.Intn(100)}
		},
		Func: func(ctx context.Context, p pair, dm divMod) error {
			_, _, err := dm.DivMod(ctx, p.x, p.y)
			return err
		},
	})
	if results, err := sim.Simulate(context.Background()); err == nil && results.Err == nil {
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
		opts := Options{
			Seed:        entry.Seed,
			NumReplicas: entry.NumReplicas,
			NumOps:      entry.NumOps,
			FailureRate: entry.FailureRate,
			YieldRate:   entry.YieldRate,
		}
		sim := simulator(t, opts)
		RegisterOp(sim, Op[pair]{
			Name: "divmod",
			Gen: func(r *rand.Rand) pair {
				return pair{rand.Intn(100), rand.Intn(100)}
			},
			Func: func(ctx context.Context, p pair, dm divMod) error {
				_, _, err := dm.DivMod(ctx, p.x, p.y)
				return err
			},
		})
		if results, err := sim.Simulate(context.Background()); err == nil && results.Err == nil {
			t.Fatal("unexpected success")
		}
	}
}

func TestCancelledSimulation(t *testing.T) {
	// Run a blocking simulation and cancel it.
	sim := simulator(t, Options{NumReplicas: 10, NumOps: 1000})
	RegisterOp(sim, Op[struct{}]{
		Name: "block",
		Gen: func(r *rand.Rand) struct{} {
			return struct{}{}
		},
		Func: func(ctx context.Context, _ struct{}, b blocker) error {
			b.Block(ctx)
			return ctx.Err()
		},
	})

	const delay = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), delay)
	defer cancel()
	errs := make(chan error)
	go func() {
		results, err := sim.Simulate(ctx)
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

func TestFailureRateZero(t *testing.T) {
	// With FailureRate set to zero, no operation should fail.
	opts := Options{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 0.0,
		YieldRate:   0.5,
	}
	sim := simulator(t, opts)
	RegisterOp(sim, Op[pair]{
		Name: "divmod",
		Gen: func(r *rand.Rand) pair {
			return pair{1 + rand.Intn(100), 1 + rand.Intn(100)}
		},
		Func: func(ctx context.Context, p pair, dm divMod) error {
			_, _, err := dm.DivMod(ctx, p.x, p.y)
			return err
		},
	})
	results, err := sim.Simulate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if results.Err != nil {
		t.Fatal(results.Err)
	}
}

func TestFailureRateOne(t *testing.T) {
	// With FailureRate set to one, all operations should fail.
	opts := Options{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 1.0,
		YieldRate:   0.5,
	}
	sim := simulator(t, opts)
	RegisterOp(sim, Op[pair]{
		Name: "divmod",
		Gen: func(r *rand.Rand) pair {
			return pair{1 + rand.Intn(100), 1 + rand.Intn(100)}
		},
		Func: func(ctx context.Context, p pair, dm divMod) error {
			_, _, err := dm.DivMod(ctx, p.x, p.y)
			if err == nil {
				return fmt.Errorf("unexpected success")
			}
			return nil
		},
	})
	results, err := sim.Simulate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if results.Err != nil {
		t.Fatal(results.Err)
	}
}

// errorCounter is a fake implementation of the divMod component. See
// TestInjectedErrors for usage.
type errorCounter struct {
	mu       sync.Mutex
	id       int
	executed map[int]struct{}
	errored  map[int]struct{}
}

func (e *errorCounter) DivMod(_ context.Context, x, _ int) (int, int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executed[x] = struct{}{}
	return 0, 0, nil
}

func (e *errorCounter) NextId() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.id++
	return e.id
}

func (e *errorCounter) Errored(x int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errored[x] = struct{}{}
}

func TestInjectedErrors(t *testing.T) {
	counter := &errorCounter{
		executed: map[int]struct{}{},
		errored:  map[int]struct{}{},
	}

	// An injected RemoteCallError can happen before or after a call is
	// executed. Record the calls that error out without executing and the
	// calls that error out after executing. Both should happen.
	opts := Options{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 0.5,
		YieldRate:   0.5,
		Fakes:       map[reflect.Type]any{reflection.Type[divMod](): counter},
	}
	sim := simulator(t, opts)
	RegisterOp(sim, Op[struct{}]{
		Name: "divmod",
		Gen:  func(r *rand.Rand) struct{} { return struct{}{} },
		Func: func(ctx context.Context, _ struct{}, dm divMod) error {
			id := counter.NextId()
			_, _, err := dm.DivMod(ctx, id, id)
			if errors.Is(err, weaver.RemoteCallError) {
				counter.Errored(id)
			}
			return nil
		},
	})
	if results, err := sim.Simulate(context.Background()); err != nil || results.Err != nil {
		t.Fatal(err)
	}

	failedWithoutExecuting := 0
	failedAfterExecuting := 0
	for id := range counter.errored {
		if _, ok := counter.executed[id]; ok {
			failedAfterExecuting++
		} else {
			failedWithoutExecuting++
		}
	}
	if failedWithoutExecuting == 0 {
		t.Error("no calls failed without executing")
	}
	if failedAfterExecuting == 0 {
		t.Error("no calls failed after executing")
	}
}

type fakeDivMod struct{}

func (fakeDivMod) DivMod(context.Context, int, int) (int, int, error) {
	return 42, 42, nil
}

func TestFakes(t *testing.T) {
	opts := Options{
		NumReplicas: 10,
		NumOps:      1000,
		YieldRate:   0.5,
		Fakes: map[reflect.Type]any{
			reflection.Type[divMod](): fakeDivMod{},
		},
	}
	sim := simulator(t, opts)
	RegisterOp(sim, Op[pair]{
		Name: "divmod",
		Gen: func(r *rand.Rand) pair {
			return pair{1 + rand.Intn(100), 1 + rand.Intn(100)}
		},
		Func: func(ctx context.Context, p pair, dm divMod) error {
			div, mod, err := dm.DivMod(ctx, p.x, p.y)
			if err != nil {
				// Ignore errors.
				return nil
			}
			if div != 42 {
				return fmt.Errorf("div %d/%d: got %d, want %d", p.x, p.y, div, 42)
			}
			if mod != 42 {
				return fmt.Errorf("mod %d%%%d: got %d, want %d", p.x, p.y, mod, 42)
			}
			return nil
		},
	})
	if results, err := sim.Simulate(context.Background()); err != nil || results.Err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateOp(t *testing.T) {
	sim := simulator(t, Options{NumReplicas: 2, NumOps: 100})
	sim.ops["foo"] = op{}
	validateOp(sim, Op[int]{
		Name: "foo",
		Gen:  func(*rand.Rand) int { return 42 },
		Func: func(*rand.Rand) error { return nil },
	})
}

func TestValidateOp(t *testing.T) {
	ops := map[string]Op[int]{
		"missing name": {
			Name: "",
			Gen:  func(*rand.Rand) int { return 42 },
			Func: func(context.Context, int) error { return nil },
		},
		"nil gen": {
			Name: "",
			Func: func(context.Context, int) error { return nil },
		},
		"nil func": {
			Name: "foo",
			Gen:  func(*rand.Rand) int { return 42 },
		},
		"func is not a function": {
			Name: "foo",
			Gen:  func(*rand.Rand) int { return 42 },
			Func: 42,
		},
		"func has too few arguments": {
			Name: "foo",
			Gen:  func(*rand.Rand) int { return 42 },
			Func: func(context.Context) error { return nil },
		},
		"func has incorrect first argument": {
			Name: "foo",
			Gen:  func(*rand.Rand) int { return 42 },
			Func: func(int, int) error { return nil },
		},
		"func has incorrect second argument": {
			Name: "foo",
			Gen:  func(*rand.Rand) int { return 42 },
			Func: func(context.Context, bool) error { return nil },
		},
		"func has incorrect third argument": {
			Name: "foo",
			Gen:  func(*rand.Rand) int { return 42 },
			Func: func(context.Context, int, int) error { return nil },
		},
		"func has incorrect number of returns": {
			Name: "foo",
			Gen:  func(*rand.Rand) int { return 42 },
			Func: func(context.Context, int) (int, error) { return 42, nil },
		},
		"func has incorrect return": {
			Name: "foo",
			Gen:  func(*rand.Rand) int { return 42 },
			Func: func(context.Context, int) int { return 42 },
		},
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			opts := Options{NumReplicas: 2, NumOps: 100}
			if _, err := validateOp(simulator(t, opts), op); err == nil {
				t.Fatal("unexpected success")
			}
		})
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
