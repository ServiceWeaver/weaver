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
	"encoding"
	"encoding/binary"
	"fmt"
	"math"

	"google.golang.org/protobuf/proto"
)

// encoderError is the type of error passed to panic by encoding code that encounters an error.
type encoderError struct {
	err error
}

func (e encoderError) Error() string {
	if e.err == nil {
		return "encoder:"
	}
	return "encoder: " + e.err.Error()
}

func (e encoderError) Unwrap() error {
	return e.err
}

// Encoder serializes data in a byte slice data.
type Encoder struct {
	data  []byte    // Contains the serialized arguments.
	space [100]byte // Prellocated buffer to avoid allocations for small size arguments.
}

func NewEncoder() *Encoder {
	var enc Encoder
	enc.data = enc.space[:0] // Arrange to use builtin buffer
	return &enc
}

// Reset resets the Encoder to use a buffer with a capacity of at least the
// provided size. All encoded data is lost.
func (e *Encoder) Reset(n int) {
	// TODO(mwhittaker): Have a NewEncoder method that takes in an initial
	// buffer? Or at least an initial capacity? And then pipe that through
	// NewCaller.
	if n <= cap(e.data) {
		e.data = e.data[:0]
	} else {
		e.data = make([]byte, 0, n)
	}
}

// makeEncodeError creates and returns an encoder error.
func makeEncodeError(format string, args ...interface{}) encoderError {
	return encoderError{fmt.Errorf(format, args...)}
}

// EncodeProto serializes value into a byte slice using proto serialization.
func (e *Encoder) EncodeProto(value proto.Message) {
	enc, err := proto.Marshal(value)
	if err != nil {
		panic(makeEncodeError("error encoding to proto %T: %w", value, err))
	}
	e.Bytes(enc)
}

// EncodeBinaryMarshaler serializes value into a byte slice using its
// MarshalBinary method.
func (e *Encoder) EncodeBinaryMarshaler(value encoding.BinaryMarshaler) {
	enc, err := value.MarshalBinary()
	if err != nil {
		panic(makeEncodeError("error encoding BinaryMarshaler %T: %w", value, err))
	}
	e.Bytes(enc)
}

// Data returns the byte slice that contains the serialized arguments.
func (e *Encoder) Data() []byte {
	return e.data
}

// Grow increases the size of the encoder's data if needed. Only appends a new
// slice if there is not enough capacity to satisfy bytesNeeded.
// Returns the slice fragment that contains bytesNeeded.
func (e *Encoder) Grow(bytesNeeded int) []byte {
	n := len(e.data)
	if cap(e.data)-n >= bytesNeeded {
		e.data = e.data[:n+bytesNeeded] // Grow in place (common case)
	} else {
		// Create a new larger slice.
		e.data = append(e.data, make([]byte, bytesNeeded)...)
	}
	return e.data[n:]
}

// Uint8 encodes an arg of type uint8.
func (e *Encoder) Uint8(arg uint8) {
	e.Grow(1)[0] = arg
}

// Byte encodes an arg of type byte.
func (e *Encoder) Byte(arg byte) {
	e.Uint8(arg)
}

// Int8 encodes an arg of type int8.
func (e *Encoder) Int8(arg int8) {
	e.Uint8(byte(arg))
}

// Uint16 encodes an arg of type uint16.
func (e *Encoder) Uint16(arg uint16) {
	binary.LittleEndian.PutUint16(e.Grow(2), arg)
}

// Int16 encodes an arg of type int16.
func (e *Encoder) Int16(arg int16) {
	e.Uint16(uint16(arg))
}

// Uint32 encodes an arg of type uint32.
func (e *Encoder) Uint32(arg uint32) {
	binary.LittleEndian.PutUint32(e.Grow(4), arg)
}

// Int32 encodes an arg of type int32.
func (e *Encoder) Int32(arg int32) {
	e.Uint32(uint32(arg))
}

// Rune encodes an arg of type rune.
func (e *Encoder) Rune(arg rune) {
	e.Int32(arg)
}

// Uint64 encodes an arg of type uint64.
func (e *Encoder) Uint64(arg uint64) {
	binary.LittleEndian.PutUint64(e.Grow(8), arg)
}

// Int64 encodes an arg of type int64.
func (e *Encoder) Int64(arg int64) {
	e.Uint64(uint64(arg))
}

// Uint encodes an arg of type uint.
// Uint can have 32 bits or 64 bits based on the machine type. To simplify our
// reasoning, we encode the highest possible value.
func (e *Encoder) Uint(arg uint) {
	e.Uint64(uint64(arg))
}

// Int encodes an arg of type int.
// Int can have 32 bits or 64 bits based on the machine type. To simplify our
// reasoning, we encode the highest possible value.
func (e *Encoder) Int(arg int) {
	e.Uint64(uint64(arg))
}

// Bool encodes an arg of type bool.
// Serialize boolean values as an uint8 that encodes either 0 or 1.
func (e *Encoder) Bool(arg bool) {
	if arg {
		e.Uint8(1)
	} else {
		e.Uint8(0)
	}
}

// Float32 encodes an arg of type float32.
func (e *Encoder) Float32(arg float32) {
	binary.LittleEndian.PutUint32(e.Grow(4), math.Float32bits(arg))
}

// Float64 encodes an arg of type float64.
func (e *Encoder) Float64(arg float64) {
	binary.LittleEndian.PutUint64(e.Grow(8), math.Float64bits(arg))
}

// Complex64 encodes an arg of type complex64.
// We encode the real and the imaginary parts one after the other.
func (e *Encoder) Complex64(arg complex64) {
	e.Float32(real(arg))
	e.Float32(imag(arg))
}

// Complex128 encodes an arg of type complex128.
func (e *Encoder) Complex128(arg complex128) {
	e.Float64(real(arg))
	e.Float64(imag(arg))
}

// String encodes an arg of type string.
// For a string, we encode its length, followed by the serialized content.
func (e *Encoder) String(arg string) {
	n := len(arg)
	if n > math.MaxUint32 {
		panic(makeEncodeError("unable to encode string; length doesn't fit in 4 bytes"))
	}
	data := e.Grow(4 + n)
	binary.LittleEndian.PutUint32(data, uint32(n))
	copy(data[4:], arg)
}

// Bytes encodes an arg of type []byte.
// For a byte slice, we encode its length, followed by the serialized content.
// If the slice is nil, we encode length as -1.
func (e *Encoder) Bytes(arg []byte) {
	if arg == nil {
		e.Int32(-1)
		return
	}
	n := len(arg)
	if n > math.MaxUint32 {
		panic(makeEncodeError("unable to encode bytes; length doesn't fit in 4 bytes"))
	}
	data := e.Grow(4 + n)
	binary.LittleEndian.PutUint32(data, uint32(n))
	copy(data[4:], arg)
}

// Len attempts to encode l as an int32.
//
// Panics if l is bigger than an int32 or a negative length (except -1).
//
// NOTE that this method should be called only in the generated code, to avoid
// generating repetitive code that encodes the length of a non-basic type (e.g., slice, map).
func (e *Encoder) Len(l int) {
	if l < -1 {
		panic(makeEncodeError("unable to encode a negative length %d", l))
	}
	if l > math.MaxUint32 {
		panic(makeEncodeError("length can't be represented in 4 bytes"))
	}
	e.Int32(int32(l))
}

// Error encoding
//
// An error can be composed of a tree of errors (see the errors package)
// We encode such a tree in a linear list of errors visited in depth-first
// order.
//
// The encoding is a list of encoded errors terminated by a <endOfErrors>.  An
// encoded error is one of:
//
// <serializedErrorVal,typKey,serial> for registered serializable error types.
//
// <serializedErrorPtr,typeKey,serial> where a pointer to an error has been
// registered as serializable.
//
// <emulatedError,message,fmtError> for unregistered error types.
const (
	endOfErrors        uint8 = 0
	serializedErrorVal uint8 = 1
	serializedErrorPtr uint8 = 2
	emulatedError      uint8 = 3
)

// Error encodes an arg of type error. We save enough type information
// to allow errors.Unwrap(), errors.Is(), and errors.As() to work correctly.
func (e *Encoder) Error(err error) {
	// Convert the tree of wrapped errors into a single list.
	// We do not use errors.Unwrap() since it does not visit the
	// children found by "Unwrap()[]error".
	//
	// The encoding is a list of encoded errors terminated by a <0>.
	// An encoded error is either <1,typKey,serialization> for
	// registered custom error types, or <2,message,fmtError> for
	// unregistered error types.
	seen := map[error]struct{}{}
	var dfs func(error)
	dfs = func(err error) {
		if err == nil {
			return
		}

		// Avoid cycles and potential exponential expansion of diamond patterns.
		if _, ok := seen[err]; ok {
			return
		}
		seen[err] = struct{}{}

		// If err can be marshaled, do that and skip extracting its children
		// since serialized form should contain all of them.
		if am, ok := err.(AutoMarshal); ok {
			e.Uint8(serializedErrorVal)
			e.Interface(am)
			return
		}

		// See if pointer to err implements AutoMarshal. This allows
		// the Error() method to have a value receiver.
		if am, ok := pointerTo(err).(AutoMarshal); ok {
			e.Uint8(serializedErrorPtr)
			e.Interface(am)
			return
		}

		// Send the error message and the representation of err so we
		// can do plain comparisons at the other end.
		e.Uint8(emulatedError)
		e.String(err.Error())
		e.String(fmtError(err))

		switch u := err.(type) {
		case interface{ Unwrap() error }:
			dfs(u.Unwrap())
		case interface{ Unwrap() []error }:
			for _, child := range u.Unwrap() {
				dfs(child)
			}
		}
	}
	dfs(err)
	e.Uint8(endOfErrors)
}

// Interface encodes value prefixed with its concrete type.
func (e *Encoder) Interface(value AutoMarshal) {
	e.String(typeKey(value))
	value.WeaverMarshal(e)
}
