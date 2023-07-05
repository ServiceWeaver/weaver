package sim

import (
	"fmt"
	"math/rand"

	"golang.org/x/exp/constraints"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

func flip(r *rand.Rand) bool {
	return r.Intn(2) == 0
}

func pop[T any](r *rand.Rand, xs []T) (T, []T) {
	if len(xs) == 0 {
		panic(fmt.Errorf("pop: no options given"))
	}
	i := r.Intn(len(xs))
	x := xs[i]
	return x, append(xs[:i], xs[i+1:]...)
}

func pick[T any](r *rand.Rand, xs []T) T {
	if len(xs) == 0 {
		panic(fmt.Errorf("pick: no options given"))
	}
	return xs[r.Intn(len(xs))]
}

func pickKey[K constraints.Ordered, V any](r *rand.Rand, m map[K]V) K {
	keys := maps.Keys(m)
	slices.Sort(keys)
	return pick(r, keys)
}
