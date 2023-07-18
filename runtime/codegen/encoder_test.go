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
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// Set of values used in tests.
var values = []interface{}{
	uint8(0), uint8(100), uint8(255),
	int8(-128), int8(0), int8(127),
	uint16(0), uint16(1056), uint16(65535),
	int16(-32768), int16(0), int16(32767),
	uint32(0), uint32(1213441242), uint32(4294967295),
	int32(-2147483648), int32(0), int32(2147483647),
	uint64(0), uint64(10246144073779551699), uint64(18446744073709551615),
	int64(-9023372036854775808), int64(0), int64(9223372036854775807),
	uint(0), uint(1024), uint(4294967295),
	int(-1024), int(0), int(9223372036854775807),
	true, false,
	float32(-21474.83648), float32(0), float32(2147483.641),
	float64(-9023372.036854775808), float64(0), float64(9223372036.4775807),
	complex(float32(1234.567), float32(5678.123)),
	complex(float64(5678.123), float64(1234.567)),
	"string to test de(serialization)", "",
	[]byte{0, 1, 1, 0, 1, 0, 0, 1}, []byte{}, []byte(nil),
}

// newEncoder instantiates a new Encoder with a preallocated space.
func newEncoder() Encoder {
	enc := Encoder{}
	enc.data = enc.space[:0]
	return enc
}

// TestReset calls the Reset method on an encoder. Verify that the buffer has
// the appropriate size and capacity.
func TestReset(t *testing.T) {
	for _, n := range []int{0, 10, 100, 1000, 10000} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			enc := newEncoder()
			enc.String("this is garbage text that will get reset")
			enc.Reset(n)
			if got, want := len(enc.data), 0; got != want {
				t.Fatalf("len(enc.data): got %d, want %d", got, want)
			}
			if got, want := cap(enc.data), n; got < want {
				t.Fatalf("cap(enc.data): got %d, want at least %d", got, want)
			}
		})
	}
}

// TestEncodeDecode encodes a value and then decodes it. Verify that the value
// is decoded as expected.
func TestEncodeDecode(t *testing.T) {
	for _, val := range values {
		input := []interface{}{val}
		enc := newEncoder()
		encode(&enc, input)

		dec := Decoder{data: enc.data}
		output := decode(&dec, input)

		if diff := cmp.Diff(input, output); diff != "" {
			t.Fatalf("list: (-want,+got):\n%s\n", diff)
		}
		if len(dec.data) > 0 {
			t.Fatalf("unexpected bytes left to be read:%d\n", len(dec.data))
		}
	}
}

// TestEncodeDecodeRandom encodes a number of random values. Verify that the
// values are decoded as expected.
func TestEncodeDecodeRandom(t *testing.T) {
	seed := rand.NewSource(time.Now().UnixNano())
	rand := rand.New(seed)

	// Generate 100 encoding/decoding runs. For every run, encode 1 to 5000
	// parameters that take random values from values.
	for run := 0; run < 100; run++ {
		n := rand.Intn(5000)
		var input []interface{}
		for i := 0; i < n; i++ {
			input = append(input, values[rand.Intn(len(values))])
		}

		enc := newEncoder()
		encode(&enc, input)

		dec := Decoder{data: enc.data}
		output := decode(&dec, input)

		if diff := cmp.Diff(input, output); diff != "" {
			t.Fatalf("list: (-want,+got):\n%s\n", diff)
		}
		if len(dec.data) > 0 {
			t.Fatalf("unexpected bytes left to be read:%d\n", len(dec.data))
		}
	}
}

// convertCallPanicToError catches and returns errors detected during fn's execution.
func convertCallPanicToError(fn func()) (err error) {
	defer func() { err = CatchPanics(recover()) }()
	fn()
	return
}

// TestErrorDecUnableToRead encodes an integer and attempts to decode an integer
// and a bool value. Verify that a decoding error is triggered because there are
// not enough bytes encoded to decode both values.
func TestErrorDecUnableToRead(t *testing.T) {
	err := convertCallPanicToError(func() {
		enc := newEncoder()
		enc.Int(12345)

		dec := Decoder{enc.data}
		dec.Int()
		dec.Bool()
	})
	if !strings.Contains(err.Error(), "unable to read #bytes") {
		t.Fatalf(err.Error())
	}
}

// TestErrorUnableToDecBool encodes an integer that is not 0 or 1 and attempts
// to decode a bool value. Verify that a decoding error is triggered because
// the boolean value can't be decoded.
func TestErrorUnableToDecBool(t *testing.T) {
	err := convertCallPanicToError(func() {
		enc := newEncoder()
		enc.Int(123)

		dec := Decoder{enc.data}
		dec.Bool()
	})
	if !strings.Contains(err.Error(), "unable to decode bool") {
		t.Fatalf(err.Error())
	}
}

// TestErrorUnableToDecBytes encodes a negative number that is not -1 and
// attempts to decode a byte slice. Verify that a decoding error is triggered
// because the length read is negative and not -1.
func TestErrorUnableToDecBytes(t *testing.T) {
	err := convertCallPanicToError(func() {
		enc := newEncoder()
		enc.Int(-10)

		dec := Decoder{enc.data}
		dec.Bytes()
	})
	if !strings.Contains(err.Error(), "unable to decode bytes; expected length") {
		t.Fatalf(err.Error())
	}
}

// Some custom error types. There are manually made serializable since we do
// not want this package to depend on the code generator.

type customTestError struct{ f string }

func (c customTestError) Error() string               { return fmt.Sprintf("custom(%s)", c.f) }
func (c *customTestError) WeaverMarshal(e *Encoder)   { e.String(c.f) }
func (c *customTestError) WeaverUnmarshal(d *Decoder) { c.f = d.String() }

// For a change, make the pointer type an error.
type alternateError struct{ f string }

func (a *alternateError) Error() string              { return fmt.Sprintf("alt(%s)", a.f) }
func (a *alternateError) WeaverMarshal(e *Encoder)   { e.String(a.f) }
func (a *alternateError) WeaverUnmarshal(d *Decoder) { a.f = d.String() }
func (a *alternateError) Is(target error) bool {
	x, ok := target.(*alternateError)
	return ok && a.f == x.f
}

type cyclicError struct{ msg string }

func (c *cyclicError) Error() string              { return c.msg }
func (c *cyclicError) Unwrap() error              { return c }
func (c *cyclicError) WeaverMarshal(e *Encoder)   { e.String(c.msg) }
func (c *cyclicError) WeaverUnmarshal(d *Decoder) { c.msg = d.String() }

func init() {
	RegisterSerializable[*customTestError]()
	RegisterSerializable[*alternateError]()
	RegisterSerializable[*cyclicError]()
}

func TestErrorValues(t *testing.T) {
	type testCase struct {
		name  string
		val   error
		is    []error // Error values target for which errors.Is(decoded, target) must be true
		notis []error // Error values target for which errors.Is(decoded, target) must be false
	}
	for _, c := range []testCase{
		{"nil", nil, nil, nil},
		{"flat", errors.New("hello"), nil, []error{os.ErrNotExist}},
		{"wrap1", fmt.Errorf("hello %w", os.ErrNotExist), []error{os.ErrNotExist}, nil},
		{"wrap2", fmt.Errorf("hello %w", fmt.Errorf("world %w", os.ErrNotExist)), []error{os.ErrNotExist}, nil},
		{"custom", customTestError{"x"}, nil, []error{os.ErrNotExist, &alternateError{"x"}}},
		{"wrap-custom", fmt.Errorf("hello %w", customTestError{"a"}), []error{customTestError{"a"}}, nil},
		{"wrap-two", fmt.Errorf("hello %w %w", customTestError{"a"}, &alternateError{"b"}), []error{customTestError{"a"}, &alternateError{"b"}}, []error{customTestError{"other"}, &alternateError{"other"}}},
		{"ptr", &alternateError{"a"}, []error{&alternateError{"a"}}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Encode/decode and get resulting error value.
			src := c.val
			enc := newEncoder()
			enc.Error(src)
			dec := Decoder{data: enc.data}
			dst := dec.Error()
			if !dec.Empty() {
				t.Fatalf("leftover bytes in decoder")
			}

			if src == nil && dst != nil {
				t.Errorf("decoded non-nil error (%v) for nil source", dst)
			}
			if src != nil && dst == nil {
				t.Errorf("decoded nil error for non-nil source (%v)", src)
			}

			// Check errors.Is(dst) for all error types that should match.
			for _, target := range c.is {
				if !errors.Is(dst, target) {
					t.Errorf("decoded error (%v) (from %v) does not match target (%v)", dst, src, target)
				}
			}

			// Check errors.Is(dst) for all error types that should not match.
			for _, target := range c.notis {
				if errors.Is(dst, target) {
					t.Errorf("decoded error (%v) (from %v) unexpectedly matches target (%v)", dst, src, target)
				}
			}

			// Checks errors.Is(dst) against all unwrappings of src, including src itself.
			for u := src; u != nil; u = errors.Unwrap(u) {
				if !errors.Is(dst, u) {
					t.Errorf("decoded error (%#v) (from %#v) does not match unwrapped error (%#v)", dst, src, u)
				}
			}
		})
	}
}

func TestCyclicError(t *testing.T) {
	// Special test for cyclic errors since errors.Is etc. can get
	// into an infinite loop on cycles.
	src := &cyclicError{"c"}
	enc := newEncoder()
	enc.Error(src)
	dec := Decoder{data: enc.data}
	dst := dec.Error()
	if !dec.Empty() {
		t.Fatalf("leftover bytes in decoder")
	}

	c, ok := dst.(*cyclicError)
	if !ok {
		t.Fatalf("did not encode cyclic error")
	}
	if c.msg != "c" {
		t.Fatalf("wrong contents for cyclic error")
	}
}

// encode serializes args using the encoder enc.
func encode(enc *Encoder, args []interface{}) {
	for _, elem := range args {
		val := reflect.ValueOf(elem).Interface()
		switch reflect.ValueOf(elem).Kind() {
		case reflect.Uint8:
			enc.Uint8(val.(uint8))
		case reflect.Int8:
			enc.Int8(val.(int8))
		case reflect.Uint16:
			enc.Uint16(val.(uint16))
		case reflect.Int16:
			enc.Int16(val.(int16))
		case reflect.Uint32:
			enc.Uint32(val.(uint32))
		case reflect.Int32:
			enc.Int32(val.(int32))
		case reflect.Uint64:
			enc.Uint64(val.(uint64))
		case reflect.Int64:
			enc.Int64(val.(int64))
		case reflect.Uint:
			enc.Uint(val.(uint))
		case reflect.Int:
			enc.Int(val.(int))
		case reflect.Bool:
			enc.Bool(val.(bool))
		case reflect.Float32:
			enc.Float32(val.(float32))
		case reflect.Float64:
			enc.Float64(val.(float64))
		case reflect.Complex64:
			enc.Complex64(val.(complex64))
		case reflect.Complex128:
			enc.Complex128(val.(complex128))
		case reflect.String:
			enc.String(val.(string))
		case reflect.Slice:
			v := reflect.TypeOf(elem).Elem().Kind()
			if v == reflect.Uint8 {
				enc.Bytes(val.([]byte))
				break
			}
			panic(fmt.Errorf("unsupported type to serialize a slice: %v", v))
		default:
			panic(fmt.Errorf("unsupported type to serialize: %v", elem))
		}
	}
}

// decode invokes the decoder dec to deserialize results based on types
// provided by out.
func decode(dec *Decoder, out []interface{}) []interface{} {
	var results []interface{}
	for _, elem := range out {
		if reflect.ValueOf(elem).Kind() == reflect.Ptr {
			elem = reflect.Indirect(reflect.ValueOf(elem)).Interface()
		}

		switch reflect.ValueOf(elem).Kind() {
		case reflect.Uint8:
			results = append(results, dec.Uint8())
		case reflect.Int8:
			results = append(results, dec.Int8())
		case reflect.Uint16:
			results = append(results, dec.Uint16())
		case reflect.Int16:
			results = append(results, dec.Int16())
		case reflect.Uint32:
			results = append(results, dec.Uint32())
		case reflect.Int32:
			results = append(results, dec.Int32())
		case reflect.Uint64:
			results = append(results, dec.Uint64())
		case reflect.Int64:
			results = append(results, dec.Int64())
		case reflect.Uint:
			results = append(results, dec.Uint())
		case reflect.Int:
			results = append(results, dec.Int())
		case reflect.Bool:
			results = append(results, dec.Bool())
		case reflect.Float32:
			results = append(results, dec.Float32())
		case reflect.Float64:
			results = append(results, dec.Float64())
		case reflect.Complex64:
			results = append(results, dec.Complex64())
		case reflect.Complex128:
			results = append(results, dec.Complex128())
		case reflect.String:
			results = append(results, dec.String())
		case reflect.Slice:
			v := reflect.TypeOf(elem).Elem().Kind()
			if v == reflect.Uint8 {
				results = append(results, dec.Bytes())
				break
			}
			panic(fmt.Errorf("unsupported type to deserialize a slice: %v", v))
		default:
			panic(fmt.Errorf("unsupported type to deserialize: %v", elem))
		}
	}
	return results
}
