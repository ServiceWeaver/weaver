package calc

import (
	"context"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/sim"
)

type pair struct {
	x, y int
}

func newPair(x, y int) pair {
	return pair{x, y}
}

var genPair = sim.Map2(
	sim.Int(-10, 10),
	sim.Int(-10, 10),
	newPair,
)

func FuzzCalc(f *testing.F) {
	for seed := 0; seed < 100; seed++ {
		f.Add(int64(seed))
	}
	f.Fuzz(func(t *testing.T, seed int64) {
		opts := sim.Options{
			Seed:        seed,
			NumOps:      3,
			NumReplicas: 1,
		}
		s := sim.New(opts)
		sim.RegisterComponent[adder](s)
		sim.RegisterComponent[multiplier](s)
		sim.RegisterComponent[calc](s)
		sim.RegisterOp(s, "add", genPair, func(c sim.Caller, ctx context.Context, p pair) error {
			c.Call("calc", "Add", ctx, p.x, p.y)
			return nil
		})
		sim.RegisterOp(s, "multiply", genPair, func(c sim.Caller, ctx context.Context, p pair) error {
			c.Call("calc", "Multiply", ctx, p.x, p.y)
			return nil
		})
		result := s.Simulate()
		if result.Err != nil {
			t.Log(s.Mermaid())
			t.Fatal(result.Err)
		}
	})
}
