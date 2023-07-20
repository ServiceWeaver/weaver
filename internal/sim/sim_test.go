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
	"math/rand"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/reflection"
)

func simulator(t *testing.T) *Simulator {
	t.Helper()
	sim, err := New(Options{NumReplicas: 2})
	if err != nil {
		t.Fatal(err)
	}
	return sim
}

func TestSim(t *testing.T) {
	type pair struct {
		x, y int
	}

	sim := simulator(t)
	x, err := sim.GetIntf(reflection.Type[swapper]())
	if err != nil {
		t.Fatal(err)
	}
	RegisterOp(sim, Op[pair]{
		Name: "swap",
		Gen: func(r *rand.Rand) pair {
			return pair{rand.Intn(100), rand.Intn(100)}
		},
		Func: func(ctx context.Context, p pair, s swapper) error {
			_, _, err := s.Swap(ctx, p.x, p.y)
			return err
		},
	})
	swapper := x.(swapper)
	x, y, err := swapper.Swap(context.Background(), 1, 2)
	if x != 2 {
		t.Errorf("x: got %d, want 2", x)
	}
	if y != 1 {
		t.Errorf("y: got %d, want 1", y)
	}
	if err != (swapperError{}) {
		t.Errorf("err: got %v, want %v", err, swapperError{})
	}
}

func TestDuplicateOp(t *testing.T) {
	sim := simulator(t)
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
			if _, err := validateOp(simulator(t), op); err == nil {
				t.Fatal("unexpected success")
			}
		})
	}
}
