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
	"errors"
	"fmt"
	"math"
	"reflect"

	"google.golang.org/protobuf/proto"
)

// decoderError is the type of error passed to panic by decoding code that encounters an error.
type decoderError struct {
	err error
}

func (d decoderError) Error() string {
	if d.err == nil {
		return "decoder:"
	}
	return "decoder: " + d.err.Error()
}

func (d decoderError) Unwrap() error {
	return d.err
}

// Decoder deserializes data from a byte slice data in the expected results.
type Decoder struct {
	data []byte
}

// NewDecoder instantiates a new Decoder for a given byte slice.
func NewDecoder(data []byte) *Decoder {
	return &Decoder{data}
}

// Empty returns true iff all bytes in d have been consumed.
func (d *Decoder) Empty() bool {
	return len(d.data) == 0
}

// makeDecodeError creates and returns a decoder error.
func makeDecodeError(format string, args ...interface{}) decoderError {
	return decoderError{fmt.Errorf(format, args...)}
}

// DecodeProto deserializes the value from a byte slice using proto serialization.
func (d *Decoder) DecodeProto(value proto.Message) {
	if err := proto.Unmarshal(d.Bytes(), value); err != nil {
		panic(makeDecodeError("error decoding to proto %T: %w", value, err))
	}
}

// DecodeBinaryUnmarshaler deserializes the value from a byte slice using
// UnmarshalBinary.
func (d *Decoder) DecodeBinaryUnmarshaler(value encoding.BinaryUnmarshaler) {
	if err := value.UnmarshalBinary(d.Bytes()); err != nil {
		panic(makeDecodeError("error decoding BinaryUnmarshaler %T: %w", value, err))
	}
}

// Read reads and returns n bytes from the decoder and advances the decode past
// the read bytes.
func (d *Decoder) Read(n int) []byte {
	if len := len(d.data); len < n {
		panic(makeDecodeError("unable to read #bytes: %d", n))
	}
	b := d.data[:n]
	d.data = d.data[n:]
	return b
}

// Uint8 decodes a value of type uint8.
func (d *Decoder) Uint8() uint8 {
	return d.Read(1)[0]
}

// Byte decodes a value of type byte.
func (d *Decoder) Byte() byte {
	return d.Uint8()
}

// Int8 decodes a value of type int8.
func (d *Decoder) Int8() int8 {
	return int8(d.Uint8())
}

// Uint16 decodes a value of type uint16.
func (d *Decoder) Uint16() uint16 {
	return binary.LittleEndian.Uint16(d.Read(2))
}

// Int16 decodes a value of type int16.
func (d *Decoder) Int16() int16 {
	return int16(d.Uint16())
}

// Uint32 decodes a value of type uint32.
func (d *Decoder) Uint32() uint32 {
	return binary.LittleEndian.Uint32(d.Read(4))
}

// Int32 decodes a value of type int32.
func (d *Decoder) Int32() int32 {
	return int32(d.Uint32())
}

// Rune decodes a value of type rune.
func (d *Decoder) Rune() rune {
	return d.Int32()
}

// Uint64 decodes a value of type uint64.
func (d *Decoder) Uint64() uint64 {
	return binary.LittleEndian.Uint64(d.Read(8))
}

// Int64 decodes a value of type int64.
func (d *Decoder) Int64() int64 {
	return int64(d.Uint64())
}

// Uint decodes a value of type uint.
// Uint values are encoded as 64 bits.
func (d *Decoder) Uint() uint {
	return uint(d.Uint64())
}

// Int decodes a value of type int.
// Int values are encoded as 64 bits.
func (d *Decoder) Int() int {
	return int(d.Int64())
}

// Bool decodes a value of type bool.
func (d *Decoder) Bool() bool {
	if b := d.Uint8(); b == 0 {
		return false
	} else if b == 1 {
		return true
	} else {
		panic(makeDecodeError("unable to decode bool; expected {0, 1} got %v", b))
	}
}

// Float32 decodes a value of type float32.
func (d *Decoder) Float32() float32 {
	return math.Float32frombits(d.Uint32())
}

// Float64 decodes a value of type float64.
func (d *Decoder) Float64() float64 {
	return math.Float64frombits(d.Uint64())
}

// Complex64 decodes a value of type complex64.
func (d *Decoder) Complex64() complex64 {
	return complex(d.Float32(), d.Float32())
}

// Complex128 decodes a value of type complex128.
func (d *Decoder) Complex128() complex128 {
	return complex(d.Float64(), d.Float64())
}

// String decodes a value of type string.
func (d *Decoder) String() string {
	return string(d.Bytes())
}

// Bytes decodes a value of type []byte.
func (d *Decoder) Bytes() []byte {
	n := d.Int32()

	// n == -1 means a nil slice.
	if n == -1 {
		return nil
	}
	if n < 0 {
		panic(makeDecodeError("unable to decode bytes; expected length >= 0 got %d", n))
	}
	return d.Read(int(n))
}

// Len attempts to decode an int32.
//
// Panics if the result is negative (except -1).
//
// NOTE that this method should be called only in the generated code, to avoid
// generating repetitive code that decodes the length of a non-basic type (e.g., slice, map).
func (d *Decoder) Len() int {
	n := int(d.Int32())
	if n < -1 {
		panic(makeDecodeError("length can't be smaller than -1"))
	}
	return n
}

// Error decodes an error. We construct an instance of a special error value
// that provides Is and Unwrap support.
func (d *Decoder) Error() error {
	// Decode the list of errors produced by Encoder.Error().
	var list []error
	for {
		tag := d.Uint8()
		if tag == endOfErrors {
			break
		} else if tag == serializedErrorVal {
			val := d.Interface()
			if e, ok := val.(error); ok {
				list = append(list, e)
				continue
			}
			panic(fmt.Sprintf("received type %T which is not an error", val))
		} else if tag == serializedErrorPtr {
			val := d.Interface()
			if e, ok := pointee(val).(error); ok {
				list = append(list, e)
				continue
			}
			panic(fmt.Sprintf("received type %T which is not a pointer to error", val))
		} else if tag == emulatedError {
			msg := d.String()
			f := d.String()
			list = append(list, decodedError{msg, f})
		} else {
			panic(fmt.Sprintf("invalid error list tag %d", tag))
		}
	}
	if len(list) == 1 {
		return list[0] // Preserve original error instead of wrapping it via Join
	}
	return errors.Join(list...)
}

// Interface decodes a value encoded by Encoder.Interface.
// Panics if the encoded value does not belong to a type registered
// using RegisterSerializable.
func (d *Decoder) Interface() any {
	key := d.String()
	typesMu.Lock()
	defer typesMu.Unlock()
	t, ok := types[key]
	if !ok {
		panic(fmt.Sprintf("received value for non-registered type %q", key))
	}

	// Allocate space for the value.
	var ptr reflect.Value
	if t.Kind() == reflect.Pointer {
		ptr = reflect.New(t.Elem())
	} else {
		ptr = reflect.New(t)
	}
	am, ok := ptr.Interface().(AutoMarshal)
	if !ok {
		panic(fmt.Sprintf("received value for non-serializable type %v", t))
	}
	am.WeaverUnmarshal(d)

	result := ptr
	if t.Kind() != reflect.Pointer {
		result = ptr.Elem()
	}
	return result.Interface()
}

// decodedError is an error used for non-serializable decoded errors.
// It supports Error() by returning the Error() string precomputed at
// the send. It partially supports Is() by comparing the string
// representation of the value.
type decodedError struct {
	msg string // Error() result
	fmt string // Result of fmtError
}

var _ error = decodedError{}

// Error implements error.Error.
func (e decodedError) Error() string { return e.msg }

// Is returns true if the representation of this error is the same as the
// representation of the target error. Note that this is not the same
// as the normal specification of errors.Is, but is the best we can do.
func (e decodedError) Is(target error) bool {
	return e.fmt == fmtError(target)
}

// fmtError serializes an error value including its type info using fmt.Sprintf.
func fmtError(v error) string {
	// Include package and type info explicitly since %#v uses a shortened path.
	t := reflect.TypeOf(v)
	return fmt.Sprintf("(%s:%#v)", t.PkgPath(), v)
}
