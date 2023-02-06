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

package codegen

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
)

// # Overview
//
// OrderedEncoder implements a set of order preserving serialization functions.
// That is, given two values x and y and serialization function f:
//
//	x <= y iff f(x) <= f(y)
//
// There are serialization functions for unsigned integers (uint8, uint16,
// uint32, uint64, uint), signed integers (int8, int16, int32, int64, int),
// floating point numbers (float32, float64), and strings.
//
// When multiple primitive values are serialized together, the serialization
// preserves their lexicographic ordering. For example, given two tuples
//
//	t1 = (x1, ..., xn) and t2 = (x1, ..., xn)
//
// Comparing t1 and t2 lexicographically yields the same result as comparing
// the serialization of t1 and t2.
//
// # Details
//
// To understand ordered code, you have to understand (1) how every type is
// serialized individually to preserve order, (2) how lexicographic order is
// preserved when serializing multiple values, and (3) how we encode the
// Infinity value. To understand (1), see the comments next to the
// serialization functions below. To understand (3), see the initialize method
// below. Here, we explain (2)
//
// Integers (signed or unsigned) and floating point values have fixed size
// serializations. A uint32, for example, is always serialized as 32 bits.
// This makes preserving lexicographic order trivial. Strings, on the other
// hand, have variable length serializations. This makes it more challenging to
// preserve lexicographic order.
//
// For example, imagine we have a tuple type consisting of two strings. Tuple
// t1 = ("a", "xx") and tuple t2 = ("ab", "c"). Lexicographically, t1 < t2
// because "a" < "ab", but if we naively serialize the tuples as "axx" and
// "abc", then "axx" > "abc". We need to insert some sort of separator to
// indicate the end of a string.
//
// Let's use the null byte '\x00' as the separator. We serialize t1 and t2 as
// "a\x00xx" and "ab\x00c". Now, "a\x00xx" < "ab\x00c" as expected. Note that
// we use the null byte as our separator because it is ordered before all other
// bytes.  If we used a different separator, say "z", our serialization
// wouldn't work.
//
// Now consider tuples t3 = ("a", "x") and t4 = ("a\x00b", "c"). t3 < t4
// because "a" < "a\x00b", but if we serialize the two tuples, we see "a\x00x"
// > "a\x00b\x00c".  If a string contains our null byte separator, order isn't
// preserved.
//
// To solve this, we need to escape any null bytes in the provided strings. To
// do so, we replace the null byte "\x00" with two bytes "\x00\xFF" (the null
// byte followed by the byte of all ones). We also use "\x00\x00", instead of a
// plain null byte, as our separator. Now, we serialize t3 and t4 as
// "a\x00\x00x" and "a\x00\xFFb\x00\x00c" and see that "a\x00\x00x" <
// "a\x00\xFFb\x00\x00c" as expected.

// OrderedCode is an order preserving encoded value.
type OrderedCode string

// Infinity is the greatest element of the set of all OrderedCodes. That is,
// for every OrderedCode x produced by an Encoder, Infinity > x.
const Infinity OrderedCode = "\xFF"

// OrderedEncoder serializes values in an order preserving fashion. When multiple
// values are serialized together, their lexicographic ordering is preserved.
// For example,
//
//	var e orderedcode.OrderedEncoder
//	e.WriteUint8(1)
//	e.WriteFloat32(2.0)
//	e.OrderedCode()
type OrderedEncoder struct {
	buf         bytes.Buffer
	initialized bool // has the initial zero byte been written?
}

// initialize initializes an encoder. The initialize method should be called
// before any bytes are written to e.buf.
func (e *OrderedEncoder) initialize() {
	// NOTE(mwhittaker): We use an initialize function instead of a constructor
	// to allow the zero value of an Encoder to be a valid Encoder. This adds a
	// tiny bit of complexity to the implementation, but it makes it harder for
	// the users of this library to mess up.

	// It is convenient to have Infinity order after every serialized string.
	// To accomplish this, we prepend every serialization with the zero byte
	// and have Infinity be the byte "\xFF".
	if e.initialized {
		return
	}
	e.buf.WriteByte(0)
	e.initialized = true
}

// Unsigned Integers
//
// An n-byte unsigned integer is serialized as n bytes in big endian format.
// This is the most straightforward serialization. A uint, which is either 4
// bytes or 8 bytes depending on the machine, is serialized as a uint64.

// WriteUint8 serializes a value of type uint8.
func (e *OrderedEncoder) WriteUint8(x uint8) {
	e.initialize()
	e.buf.WriteByte(x)
}

// WriteUint16 serializes a value of type uint16.
func (e *OrderedEncoder) WriteUint16(x uint16) {
	e.initialize()
	var scratch [2]byte
	binary.BigEndian.PutUint16(scratch[:], x)
	e.buf.Write(scratch[:])
}

// WriteUint32 serializes a value of type uint32.
func (e *OrderedEncoder) WriteUint32(x uint32) {
	e.initialize()
	var scratch [4]byte
	binary.BigEndian.PutUint32(scratch[:], x)
	e.buf.Write(scratch[:])
}

// WriteUint64 serializes a value of type uint64.
func (e *OrderedEncoder) WriteUint64(x uint64) {
	e.initialize()
	var scratch [8]byte
	binary.BigEndian.PutUint64(scratch[:], x)
	e.buf.Write(scratch[:])
}

// WriteUint serializes a value of type uint.
func (e *OrderedEncoder) WriteUint(x uint) {
	e.initialize()
	e.WriteUint64(uint64(x))
}

// Signed Integers
//
// In go, signed integers are represented using two's complement. Consider 3
// bit signed integers. We have the following values in the range [-4, 3).
//
//     -4: 100
//     -3: 101
//     -2: 110
//     -1: 111
//      0: 000
//      1: 001
//      2: 010
//      3: 011
//
// To serialize an unsigned integer, we simply flip the first bit. Doing so
// reorders all the negative numbers before all the non-negative numbers.
//
//     -4: 100 => 000
//     -3: 101 => 001
//     -2: 110 => 010
//     -1: 111 => 011
//      0: 000 => 100
//      1: 001 => 101
//      2: 010 => 110
//      3: 011 => 111
//
// An int is serialized as an int64.

// WriteInt8 serializes a value of type int8.
func (e *OrderedEncoder) WriteInt8(x int8) {
	e.initialize()
	e.WriteUint8(uint8(x) ^ (1 << 7))
}

// WriteInt16 serializes a value of type int16.
func (e *OrderedEncoder) WriteInt16(x int16) {
	e.initialize()
	e.WriteUint16(uint16(x) ^ (1 << 15))
}

// WriteInt32 serializes a value of type int32.
func (e *OrderedEncoder) WriteInt32(x int32) {
	e.initialize()
	e.WriteUint32(uint32(x) ^ (1 << 31))
}

// WriteInt64 serializes a value of type int64.
func (e *OrderedEncoder) WriteInt64(x int64) {
	e.initialize()
	e.WriteUint64(uint64(x) ^ (1 << 63))
}

// WriteInt serializes a value of type int.
func (e *OrderedEncoder) WriteInt(x int) {
	e.initialize()
	e.WriteInt64(int64(x))
}

// Floating Point Numbers
//
// See [1] for an overview of the very ubiquitous IEEE 754 format which Go uses
// to represent floating point numbers. The format is a little complicated, but
// fortunately it was designed to preserve order when serialized (more or
// less).
//
// To understand the IEEE 754 format, let's simplify it a bit. Rather than
// looking at floating point numbers, let's look at integers in the range [-64,
// 64], but we'll format them similar to IEEE 754. For every integer, we first
// write the integer in binary scientific notation. 25, for example, is 11001
// in binary which we can write as 1.1001 x 2^4. We then store the integer as
// two bytes. The leading bit is a sign bit. The next 7 bits store the
// exponent, and the final 8 bits store the decimal (or fraction).
//
//                          fraction (1001)
//                             _______
//                            /       \
//     sign bit -> 0 000 0100 1001 0000
//                   \______/
//
//                 exponent (4)
//
// Note that to compare two positive integers formatted this way, we can simply
// compare their serialization. The reason is that we can compare two integers
// by first comparing their exponents and then comparing their fractions. A
// number with a larger exponent is necessarily larger, and for two numbers
// with the same exponent, the one with a larger fraction is necessarily
// larger. The IEEE 754 format puts the exponent first and then the fraction,
// which is exactly what we want.
//
// But what about negative numbers? To handle negative numbers, we'll use a
// trick similar to what we used for signed integers. We'll first flip the sign
// bit to serialize positive numbers after negative numbers. Then, because the
// exponent and fraction of negative numbers is not using two's complement, the
// ordering is reversed. We flip all bits in the exponent and fraction for
// negative numbers to correct this reversal. In summary, negative numbers have
// all bits flipped, while positive numbers only have their sign bit flipped.
//
// In reality, the IEE 754 format has a lot more complexity (denormalized
// numbers, positive and negative zeros, NaN's, positive and negative infinity,
// etc.), but thankfully this serialization approach handles all these
// complexities.
//
// [1]: https://en.wikipedia.org/wiki/IEEE_754-1985

// WriteFloat32 serializes a value of type float32.
func (e *OrderedEncoder) WriteFloat32(f float32) {
	e.initialize()

	// This looks funny, but if f is -0 (which is equal to 0), we want to
	// switch it to 0.
	if f == 0 {
		f = 0
	}

	bits := math.Float32bits(f)
	if f < 0 {
		e.WriteUint32(^bits)
	} else {
		e.WriteUint32(bits ^ (1 << 31))
	}
}

// WriteFloat64 serializes a value of type float64.
func (e *OrderedEncoder) WriteFloat64(f float64) {
	e.initialize()

	// This looks funny, but if f is -0 (which is equal to 0), we want to
	// switch it to 0.
	if f == 0 {
		f = 0
	}

	bits := math.Float64bits(f)
	if f < 0 {
		e.WriteUint64(^bits)
	} else {
		e.WriteUint64(bits ^ (1 << 63))
	}
}

// WriteString serializes a value of type string.
func (e *OrderedEncoder) WriteString(s string) {
	e.initialize()

	// See the comment at the top of this file. We use "\x00\x00" as our
	// separator and escape null bytes as "\x00\xFF".
	if strings.Count(s, "\x00") > 0 {
		s = strings.ReplaceAll(s, "\x00", "\x00\xFF")
	}
	e.buf.Grow(len(s) + 2)
	e.buf.WriteString(s)
	e.buf.WriteByte(0)
	e.buf.WriteByte(0)
}

// Encode returns the encoding.
func (e *OrderedEncoder) Encode() OrderedCode {
	return OrderedCode(e.buf.String())
}

// Reset resets the encoder to be empty, but retains any previously used space
// for future serialization.
func (e *OrderedEncoder) Reset() {
	e.buf.Reset()
	e.buf.WriteByte(0) // See the initialize method for details.
	e.initialized = true
}
