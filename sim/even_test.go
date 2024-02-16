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
