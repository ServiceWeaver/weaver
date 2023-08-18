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
	"testing"
	"time"
)

type pair struct {
	x, y int
}

func simulator(t *testing.T, opts Options) *Simulator {
	t.Helper()
	sim, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return sim
}

func TestSuccessfulSimulation(t *testing.T) {
	// Run the simulator, ignoring any errors. The simulation as a whole should
	// pass.
	sim := simulator(t, Options{NumReplicas: 10, NumOps: 1000})
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
	if err := sim.Simulate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestUnsuccessfulSimulation(t *testing.T) {
	// Run the simulator, erroring out if we ever have a zero denominator. The
	// simulation as a whole should fail with extremely high likelihood.
	sim := simulator(t, Options{NumReplicas: 10, NumOps: 1000})
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
	if err := sim.Simulate(context.Background()); err == nil {
		t.Fatal("unexpected success")
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
	go func() { errs <- sim.Simulate(ctx) }()

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
