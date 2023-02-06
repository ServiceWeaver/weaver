// Copyright 2022 Google LLC
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

package codegen_test

import (
	"math"
	"testing"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

// Currently, go doesn't let you run all the fuzz tests in a file. Instead, we
// suggest you run `go test -fuzz=Tuple`. Note, however, that running `go test`
// will run the fuzz tests with the seed values.

type ordered interface {
	uint8 | uint16 | uint32 | uint64 | uint |
		int8 | int16 | int32 | int64 | int |
		float32 | float64 |
		string
}

// seed seeds the provided testing.F with all pairs of the provided values.
func seed[T ordered](f *testing.F, xs ...T) {
	for i := 0; i < len(xs); i++ {
		for j := i; j < len(xs); j++ {
			f.Add(xs[i], xs[j])
		}
	}
}

// fuzzPrimitive fuzzes the encoding of primitive type T.
func fuzzPrimitive[T ordered](f *testing.F, encode func(T) codegen.OrderedCode) {
	f.Fuzz(func(t *testing.T, x, y T) {
		xs := encode(x)
		ys := encode(y)
		if x == y && xs != ys {
			t.Fatalf("%v == %v, but %x != %x", x, y, []byte(xs), []byte(ys))
		}
		if x < y && xs >= ys {
			t.Fatalf("%v < %v, but %x >= %x", x, y, []byte(xs), []byte(ys))
		}
		if x > y && xs <= ys {
			t.Fatalf("%v > %v, but %x <= %x", x, y, []byte(xs), []byte(ys))
		}
		if xs >= codegen.Infinity {
			t.Fatalf("%v (%v) >= Infinity", x, []byte(xs))
		}
		if ys >= codegen.Infinity {
			t.Fatalf("%v (%v) >= Infinity", y, []byte(ys))
		}
	})
}

func FuzzUint8(f *testing.F) {
	seed[uint8](f, 0, 1, math.MaxUint8-1, math.MaxUint8)
	fuzzPrimitive(f, func(x uint8) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteUint8(x)
		return e.Encode()
	})
}

func FuzzUint16(f *testing.F) {
	seed[uint16](f, 0, 1, math.MaxUint16-1, math.MaxUint16)
	fuzzPrimitive(f, func(x uint16) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteUint16(x)
		return e.Encode()
	})
}

func FuzzUint32(f *testing.F) {
	seed[uint32](f, 0, 1, math.MaxUint32-1, math.MaxUint32)
	fuzzPrimitive(f, func(x uint32) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteUint32(x)
		return e.Encode()
	})
}

func FuzzUint64(f *testing.F) {
	seed[uint64](f, 0, 1, math.MaxUint64-1, math.MaxUint64)
	fuzzPrimitive(f, func(x uint64) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteUint64(x)
		return e.Encode()
	})
}

func FuzzUint(f *testing.F) {
	seed[uint](f, 0, 1, math.MaxUint64-1, math.MaxUint64)
	fuzzPrimitive(f, func(x uint) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteUint(x)
		return e.Encode()
	})
}

func FuzzInt8(f *testing.F) {
	seed[int8](f, math.MinInt8, math.MinInt8+1, -1, 0, 1, math.MaxInt8-1, math.MaxInt8)
	fuzzPrimitive(f, func(x int8) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteInt8(x)
		return e.Encode()
	})
}

func FuzzInt16(f *testing.F) {
	seed[int16](f, math.MinInt16, math.MinInt16+1, -1, 0, 1, math.MaxInt16-1, math.MaxInt16)
	fuzzPrimitive(f, func(x int16) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteInt16(x)
		return e.Encode()
	})
}

func FuzzInt32(f *testing.F) {
	seed[int32](f, math.MinInt32, math.MinInt32+1, -1, 0, 1, math.MaxInt32-1, math.MaxInt32)
	fuzzPrimitive(f, func(x int32) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteInt32(x)
		return e.Encode()
	})
}

func FuzzInt64(f *testing.F) {
	seed[int64](f, math.MinInt64, math.MinInt64+1, -1, 0, 1, math.MaxInt64-1, math.MaxInt64)
	fuzzPrimitive(f, func(x int64) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteInt64(x)
		return e.Encode()
	})
}

func FuzzInt(f *testing.F) {
	seed[int](f, math.MinInt64, math.MinInt64+1, -1, 0, 1, math.MaxInt64-1, math.MaxInt64)
	fuzzPrimitive(f, func(x int) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteInt(x)
		return e.Encode()
	})
}

func FuzzFloat32(f *testing.F) {
	seed[float32](f,
		float32(math.Inf(-1)),
		-math.MaxFloat32,
		-math.MaxFloat32+1,
		-1,
		-math.SmallestNonzeroFloat32,
		float32(math.Copysign(0, -1)),
		0,
		math.SmallestNonzeroFloat32,
		1,
		math.MaxFloat32-1,
		math.MaxFloat32,
		float32(math.Inf(1)),
		float32(math.NaN()),
	)
	fuzzPrimitive(f, func(f float32) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteFloat32(f)
		return e.Encode()
	})
}

func FuzzFloat64(f *testing.F) {
	seed[float64](f,
		math.Inf(-1),
		-math.MaxFloat64,
		-math.MaxFloat64+1,
		-1,
		-math.SmallestNonzeroFloat64,
		math.Copysign(0, -1),
		0,
		math.SmallestNonzeroFloat64,
		1,
		math.MaxFloat64-1,
		math.MaxFloat64,
		math.Inf(1),
		math.NaN(),
	)
	fuzzPrimitive(f, func(f float64) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteFloat64(f)
		return e.Encode()
	})
}

func FuzzString(f *testing.F) {
	seed(f, "", "a", "\x00", "\x00\x00", "\x00\xFF")
	fuzzPrimitive(f, func(s string) codegen.OrderedCode {
		var e codegen.OrderedEncoder
		e.WriteString(s)
		return e.Encode()
	})
}

type tuple struct {
	a string
	b string
	c uint
	d string
	e int
	f string
	g float64
	h string
}

func encode(x tuple) codegen.OrderedCode {
	var e codegen.OrderedEncoder
	e.WriteString(x.a)
	e.WriteString(x.b)
	e.WriteUint(x.c)
	e.WriteString(x.d)
	e.WriteInt(x.e)
	e.WriteString(x.f)
	e.WriteFloat64(x.g)
	e.WriteString(x.h)
	return e.Encode()
}

func equal(x, y tuple) bool {
	return x.a == y.a &&
		x.b == y.b &&
		x.c == y.c &&
		x.d == y.d &&
		x.e == y.e &&
		x.f == y.f &&
		x.g == y.g &&
		x.h == y.h
}

func less(x, y tuple) bool {
	switch {
	case x.a != y.a:
		return x.a < y.a
	case x.b != y.b:
		return x.b < y.b
	case x.c != y.c:
		return x.c < y.c
	case x.d != y.d:
		return x.d < y.d
	case x.e != y.e:
		return x.e < y.e
	case x.f != y.f:
		return x.f < y.f
	case x.g != y.g:
		return x.g < y.g
	case x.h != y.h:
		return x.h < y.h
	}
	return false
}

// FuzzTuple checks that serialization preserves the lexicographic ordering of
// tuples of values.
func FuzzTuple(f *testing.F) {
	f.Add(
		"a", "b", uint(0), "", 0, "", 0.0, "",
		"a\x00", "", uint(0), "", 0, "", 0.0, "",
	)
	f.Fuzz(func(t *testing.T,
		a1 string, b1 string, c1 uint, d1 string, e1 int, f1 string, g1 float64, h1 string,
		a2 string, b2 string, c2 uint, d2 string, e2 int, f2 string, g2 float64, h2 string,
	) {
		x := tuple{a1, b1, c1, d1, e1, f1, g1, h1}
		y := tuple{a2, b2, c2, d2, e2, f2, g2, h2}
		xs := encode(x)
		ys := encode(y)
		switch {
		case equal(x, y): // x == y
			if xs != ys {
				t.Fatalf("%#v == %#v, but %x != %x", x, y, []byte(xs), []byte(ys))
			}
		case less(x, y): // x < y
			if xs >= ys {
				t.Fatalf("%#v < %#v, but %x >= %x", x, y, []byte(xs), []byte(ys))
			}
		default: // x > y
			if xs <= ys {
				t.Fatalf("%#v > %#v, but %x <= %x", x, y, []byte(xs), []byte(ys))
			}
		}
		if xs >= codegen.Infinity {
			t.Fatalf("%v (%v) >= Infinity", x, []byte(xs))
		}
		if ys >= codegen.Infinity {
			t.Fatalf("%v (%v) >= Infinity", y, []byte(ys))
		}
	})
}
