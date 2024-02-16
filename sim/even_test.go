// Copyright 2024 Google LLC
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

package sim_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/sim"
)

func even(x int) bool {
	return x%2 == 0
}

type EvenWorkload struct {
	x int
}

func (e *EvenWorkload) Init(r sim.Registrar) error {
	r.RegisterGenerators("Add", sim.Filter(sim.Int(), even))
	r.RegisterGenerators("Multiply", sim.Int())
	return nil
}

func (e *EvenWorkload) Add(_ context.Context, y int) error {
	e.x = e.x + y
	if !even(e.x) {
		return fmt.Errorf("%d is not even", e.x)
	}
	return nil
}

func (e *EvenWorkload) Multiply(_ context.Context, y int) error {
	e.x = e.x * y
	if !even(e.x) {
		return fmt.Errorf("%d is not even", e.x)
	}
	return nil
}

func TestEvenWorkload(t *testing.T) {
	s := sim.New(t, &EvenWorkload{}, sim.Options{})
	r := s.Run(5 * time.Second)
	if r.Err != nil {
		t.Log(r.Mermaid())
		t.Fatal(r.Err)
	}
}
