package calc

import (
	"context"

	"github.com/ServiceWeaver/weaver/internal/sim"
)

type adder struct {
	caller sim.Caller
}

func (a *adder) SetCaller(caller sim.Caller) {
	a.caller = caller
}

func (a *adder) Add(_ context.Context, x, y int) (int, error) {
	return x + y, nil
}

type multiplier struct {
	caller sim.Caller
}

func (m *multiplier) SetCaller(caller sim.Caller) {
	m.caller = caller
}

func (m *multiplier) Multiply(_ context.Context, x, y int) (int, error) {
	return x * y, nil
}

type calc struct {
	caller sim.Caller
}

func (c *calc) SetCaller(caller sim.Caller) {
	c.caller = caller
}

func (c *calc) Add(ctx context.Context, x, y int) (int, error) {
	sum, err := c.caller.Call("adder", "Add", ctx, x, y)
	if err != nil {
		return 0, err
	}
	return sum.(int), nil
}

func (c *calc) Multiply(ctx context.Context, x, y int) (int, error) {
	product, err := c.caller.Call("multiplier", "Multiply", ctx, x, y)
	if err != nil {
		return 0, err
	}
	return product.(int), nil
}
