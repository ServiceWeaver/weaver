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

package weaver

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/google/go-cmp/cmp"
)

type localClient struct {
	fn interface{}
}

var _ call.Connection = &localClient{}

func (c *localClient) Call(ctx context.Context, _ call.MethodKey, args []byte, _ call.CallOptions) ([]byte, error) {
	return handleCall(ctx, reflect.ValueOf(c.fn), args)
}

func (c *localClient) Close() {}

func TestCall(t *testing.T) {
	type testCase struct {
		fn      interface{}
		args    []interface{}
		results []interface{}
		err     string
	}
	for _, test := range []testCase{
		// Test method error returned.
		{
			func(context.Context) error { return errors.New("testerror") },
			nil,
			nil,
			"testerror",
		},
		// Test multiple arguments.
		{
			func(_ context.Context, a int, b string, c string) (string, error) {
				return fmt.Sprintf("%v, %v, %v", a, b, c), nil
			},
			[]interface{}{1, "str", "sstr"},
			[]interface{}{"1, str, sstr"},
			"",
		},
		// Test multiple results.
		{
			func(_ context.Context) (int, string, string, error) {
				return 1, "str", "sstr", nil
			},
			nil,
			[]interface{}{1, "str", "sstr"},
			"",
		},
		// Test too few arguments.
		{
			func(_ context.Context, a int, b string) error { return nil },
			[]interface{}{1},
			nil,
			"decoder: unable to read",
		},
		// Test wrong argument type.
		{
			func(_ context.Context, a int) error { return nil },
			[]interface{}{"str"},
			nil,
			"decoder: unable to read",
		},
		// Test too many results.
		{
			func(_ context.Context) (int, error) {
				return 1, nil
			},
			nil,
			[]interface{}{1, "str"},
			"decoder: unable to read",
		},
		// Test wrong result type.
		{
			func(_ context.Context) (string, error) {
				return "str", nil
			},
			nil,
			[]interface{}{1},
			"decoder: unable to read",
		},
		// Test encode only using the json encoder.
		{
			func(_ context.Context, a int, b string, c bool) (string, error) {
				return fmt.Sprintf("%v, %v, %v", a, b, c), nil
			},
			[]interface{}{1, "str", false},
			[]interface{}{"1, str, false"},
			"",
		},
		// Test encode only using the ServiceWeaver encoder.
		{
			func(_ context.Context, a int, b string, c bool) (string, error) {
				return fmt.Sprintf("%v, %v, %v", a, b, c), nil
			},
			[]interface{}{1, "str", false},
			[]interface{}{"1, str, false"},
			"",
		},
	} {
		err := convertCallPanicToError(func() error {
			// Encode arguments.
			enc := codegen.NewEncoder()
			for _, arg := range test.args {
				weaverEncode(enc, arg)
			}

			// Call the method.
			stub := stub{
				conn: &localClient{fn: test.fn},
				methods: []stubMethod{
					{key: call.MakeMethodKey("", "test")},
				},
			}
			out, err := stub.Run(context.Background(), 0, enc.Data(), 0)
			if err != nil {
				return err
			}

			// Decode the results.
			var testResults []interface{}
			for _, res := range test.results {
				testResults = append(testResults, reflect.New(reflect.TypeOf(res)).Interface())
			}
			var outResults []interface{}
			dec := codegen.NewDecoder(out)
			for _, res := range testResults {
				outResults = append(outResults, weaverDecode(dec, res))
			}

			if diff := cmp.Diff(test.results, outResults); diff != "" {
				t.Errorf("results (-want +got):\n%s", diff)
			}
			return nil
		})

		if test.err == "" && err != nil {
			t.Errorf("error: %v", err)
		}
		if !strings.Contains(fmt.Sprint(err), test.err) {
			t.Errorf("no substring %q in error %v", test.err, err)
		}
	}
}

// TestErrorArgsNotEncoded invokes the Run method w/o the arguments being
// encoded. Verify that a decoding error is triggered, because the receiver
// can't decode the arguments.
func TestErrorArgsNotEncoded(t *testing.T) {
	err := convertCallPanicToError(func() error {
		fn := func(_ context.Context, a int, b string) (string, error) {
			return fmt.Sprintf("%v, %v", a, b), nil
		}

		stub := stub{
			conn: &localClient{fn: fn},
			methods: []stubMethod{
				{key: call.MakeMethodKey("", "test")},
			},
		}
		_, err := stub.Run(context.Background(), 0, nil, 0)
		return err
	})

	if !strings.Contains(err.Error(), "decoder: unable to read") {
		t.Fatalf(err.Error())
	}
}

// TestErrorResultsNotDecoded invokes the Run method with the arguments being
// encoded. However, it doesn't decode the results. Verify that no result was
// decoded.
func TestErrorResultsNotDecoded(t *testing.T) {
	fn := func(_ context.Context, a int, b string) (string, error) {
		return fmt.Sprintf("%v, %v", a, b), nil
	}

	args := []interface{}{1, "str"}
	expectedResults := []interface{}{"1, str"}

	// Verify that the results are not set.
	enc := codegen.NewEncoder()
	for _, arg := range args {
		weaverEncode(enc, arg)
	}

	var testResults []interface{}
	for _, res := range expectedResults {
		testResults = append(testResults, reflect.New(reflect.TypeOf(res)).Interface())
	}

	stub := stub{
		conn: &localClient{fn: fn},
		methods: []stubMethod{
			{key: call.MakeMethodKey("", "test")},
		},
	}
	if _, err := stub.Run(context.Background(), 0, enc.Data(), 0); err != nil {
		t.Fatal(err)
	}

	for _, resPtr := range testResults {
		if val, ok := reflect.ValueOf(resPtr).Elem().Interface().(string); !ok || val != "" {
			t.Errorf("result expected to be empty; it was: %v", val)
		}
	}
}

func TestStubMethodRetries(t *testing.T) {
	reg := &codegen.Registration{
		Name: "TestInterface",
		Iface: reflection.Type[interface {
			A()
			B()
			C()
			D()
		}](),
		NoRetry: []int{1, 3},
	}
	want := []bool{true, false, true, false} // Which methods should be retriable?
	methods := makeStubMethods(reg.Name, reg)
	got := make([]bool, len(methods))
	for i, m := range methods {
		got[i] = m.retry
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("retry vector (-want,+got):\n%s\n", diff)
	}
}

// convertCallPanicToError catches and returns errors detected during fn's execution.
func convertCallPanicToError(fn func() error) (err error) {
	defer func() {
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
	}()
	return fn()
}

// handleCall invokes the method m with arguments decoded from b and returns the
// encoded results.
//
// argsEnc - contains the encoder used for each argument
// resEnc - contains the encoder used for each result
//
// NOTE: the tests support only int/string/bool encoding as of now.
func handleCall(ctx context.Context, m reflect.Value, b []byte) ([]byte, error) {
	mt := m.Type()

	// Decode arguments that arrived over the wire and make arguments for the call.
	args := make([]reflect.Value, mt.NumIn())
	args[0] = reflect.ValueOf(ctx)

	d := codegen.NewDecoder(b)
	for i := 1; i < mt.NumIn(); i++ {
		arg := reflect.New(mt.In(i)).Interface()
		args[i] = reflect.ValueOf(weaverDecode(d, arg))
	}

	// Call.
	results := m.Call(args)
	if errObject := results[len(results)-1].Interface(); errObject != nil {
		err, ok := errObject.(error)
		if !ok {
			err = errors.New("internal error")
		}
		return nil, err
	}

	// Encode results.
	e := codegen.NewEncoder()
	for i := 0; i < len(results)-1; i++ {
		weaverEncode(e, results[i].Interface())
	}
	return e.Data(), nil
}

// weaverEncode encodes input using the Service Weaver encoder.
//
// Note that the only Service Weaver encoding types supported in tests are
// int/string/bool. We should add more supported types as needed.
func weaverEncode(e *codegen.Encoder, input interface{}) {
	switch x := input.(type) {
	case int:
		e.Int(input.(int))
	case string:
		e.String(input.(string))
	case bool:
		e.Bool(input.(bool))
	default:
		panic(fmt.Errorf("Unable to encode %v (type %T) with Service Weaver encoder\n", x, x))
	}
}

// weaverDecode decodes output using the Service Weaver encoder.
//
// Note that the only Service Weaver decoding type supported in tests are
// int/string/bool. We should add more supported types as needed.
func weaverDecode(d *codegen.Decoder, output interface{}) interface{} {
	switch x := output.(type) {
	case *int:
		return d.Int()
	case *string:
		return d.String()
	case *bool:
		return d.Bool()
	default:
		panic(fmt.Errorf("Unable to decode type %v with Service Weaver decoder\n", x))
	}
}
