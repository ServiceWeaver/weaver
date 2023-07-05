package sim

import (
	"fmt"
	"math/rand"
)

// A Gen[T] generates random elements of type T.
type Gen[T any] func(*rand.Rand) T

func Return[A any](x A) Gen[A] {
	return func(*rand.Rand) A {
		return x
	}
}

func OneOf[A any](choices ...A) Gen[A] {
	if len(choices) == 0 {
		panic(fmt.Errorf("gen.OneOf: empty choices"))
	}
	return func(r *rand.Rand) A {
		return choices[r.Intn(len(choices))]
	}
}

func Combine[A any](choices ...Gen[A]) Gen[A] {
	if len(choices) == 0 {
		panic(fmt.Errorf("gen.Combine: empty choices"))
	}
	return func(r *rand.Rand) A {
		return choices[r.Intn(len(choices))](r)
	}
}

func Filter[A any](a Gen[A], f func(A) bool) Gen[A] {
	return func(r *rand.Rand) A {
		for {
			x := a(r)
			if f(x) {
				return x
			}
		}
	}
}

func Map[A, B any](a Gen[A], f func(A) B) Gen[B] {
	return func(r *rand.Rand) B {
		return f(a(r))
	}
}

func Map2[A, B, C any](a Gen[A], b Gen[B], f func(A, B) C) Gen[C] {
	return func(r *rand.Rand) C {
		return f(a(r), b(r))
	}
}

func Bind[A, B any](a Gen[A], f func(A) Gen[B]) Gen[B] {
	return func(r *rand.Rand) B {
		return f(a(r))(r)
	}
}

func Bind2[A, B, C any](a Gen[A], b Gen[B], f func(A, B) Gen[C]) Gen[C] {
	return func(r *rand.Rand) C {
		return f(a(r), b(r))(r)
	}
}

func Bool() Gen[bool] {
	return func(r *rand.Rand) bool {
		return r.Intn(2) == 0
	}
}

func Int(low, high int) Gen[int] {
	if high <= low {
		panic(fmt.Errorf("gen.Int: high (%d) <= low (%d)", high, low))
	}
	return func(r *rand.Rand) int {
		return low + r.Intn(high-low)
	}
}

func Slice[A any](elements Gen[A]) Gen[[]A] {
	return Bind(Int(0, 1000), func(n int) Gen[[]A] {
		return SizedSlice(Return(n), elements)
	})
}

func SizedSlice[A any](n Gen[int], elements Gen[A]) Gen[[]A] {
	return func(r *rand.Rand) []A {
		size := n(r)
		xs := make([]A, size)
		for i := 0; i < size; i++ {
			xs[i] = elements(r)
		}
		return xs
	}
}
