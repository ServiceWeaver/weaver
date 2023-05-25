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

package weavertest

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/private"
	"github.com/ServiceWeaver/weaver/runtime/logging"
)

// Runner runs user-supplied testing code as a weaver application.
type Runner struct {
	multi    bool // Use multiple processes
	forceRPC bool // Use RPCs even for local calls

	// Name is used as the name of the sub-test created by
	// Runner.Test (or the sub-benchmark created by
	// Runner.Bench). The default value is fine unless the user
	// has adjusted some properties of Runner like Config.
	Name string

	// Config is used as the Service Weaver configuration. The
	// string is interpreted as the contents of a Service Weaver
	// config file. It can contain application level as well as
	// component level configuration.
	Config string

	// Fakes holds a list of component implementations that should
	// be used instead of the implementations registered in the binary.
	// The typical use is to override some subset of the application
	// code being tested with test-specific component implementations.
	Fakes []FakeComponent
}

var (
	// Local is a Runner that places all components in the same process
	// and uses local procedure calls for method invocations.
	Local = Runner{Name: "Local"}

	// RPC is a Runner that places all components in the same process
	// and uses RPCs for method invocations.
	RPC = Runner{multi: false, forceRPC: true, Name: "RPC"}

	// Multi is a Runner that places all components in different
	// process (unless explicitly colocated) and uses RPCs for method
	// invocations on remote components and local procedure calls for
	// method invocations on colocated components.
	Multi = Runner{multi: true, Name: "Multi"}
)

// AllRunners returns a slice of all builtin weavertest runners.
func AllRunners() []Runner { return []Runner{Local, RPC, Multi} }

// FakeComponent records the implementation to use for a specific component type.
type FakeComponent struct {
	intf reflect.Type
	impl any
}

// Fake arranges to use impl as the implementation for the component type T.
// The result is typically placed in Runner.Fakes.
func Fake[T any](impl T) FakeComponent {
	return FakeComponent{intf: reflect.TypeOf((*T)(nil)).Elem(), impl: impl}
}

//go:generate ../cmd/weaver/weaver generate

// testMain is the component implementation used in tests.
type testMain struct {
	weaver.Implements[testMainInterface]
}

// testMainInterface is the alternative to weaver.Main we use so that
// we do not conflict with any application provided implementation of
// weaver.Main.
type testMainInterface interface{}

// Test runs a sub-test of t that tests supplied Service Weaver
// application code.  It fails at runtime if body is not a function
// whose signature looks like:
//
//	func(*testing.T, ComponentType1)
//	func(*testing.T, ComponentType, ComponentType2)
//	...
//
// body is called in the context of a brand-new Service Weaver
// application and is passed the *testing.T for the sub-test,
// followed by a the list of components.
//
//	func TestFoo(t *testing.T) {
//	    weavertest.Local.Test(t, func(t *testing.T, foo Foo, bar Bar) {
//		// Test foo and bar ...
//	    })
//	}
func (r Runner) Test(t *testing.T, body any) {
	t.Helper()
	t.Run(r.Name, func(t *testing.T) { r.sub(t, false, body) })
}

// Bench runs a sub-benchmark of b that benchmarks supplied Service
// Weaver application code.  It fails at runtime if body is not a
// function whose signature looks like:
//
//	func(*testing.B, ComponentType1)
//	func(*testing.B, ComponentType, ComponentType2)
//	...
//
// body is called in the context of a brand-new Service Weaver
// application and is passed the *testing.B for the sub-benchmark,
// followed by a the list of components.
//
//	func BenchmarkFoo(b *testing.B) {
//	    weavertest.Local.Bench(b, func(b *testing.B, foo Foo) {
//		for i := 0; i < b.N; i++ {
//		    ... use foo ...
//		}
//	    })
//	}
func (r Runner) Bench(b *testing.B, testBody any) {
	b.Helper()
	b.Run(r.Name, func(b *testing.B) { r.sub(b, true, testBody) })
}

func (r Runner) sub(t testing.TB, isBench bool, testBody any) {
	t.Helper()
	runner, err := checkRunFunc(t, testBody)
	if err != nil {
		t.Fatal(fmt.Errorf("weavertest.Run argument: %v", err))
	}

	var cleanup func() error
	ctx, cancelFn := context.WithCancel(context.Background())
	defer func() {
		// Cancel the context so background activity will stop.
		cancelFn()

		// Do any deployer specific shutdowns.
		if cleanup != nil {
			if err := cleanup(); err != nil {
				// Since we are cleaning up, the test passed and we are
				// just seeing expected shutdown errors.
				t.Log("cleanup", err)
			}
		}

		// Enable the following to print stacks of goroutine that did not shut down properly.
		if false {
			logStacks()
		}
	}()

	if !r.multi && !r.forceRPC {
		ctx = initSingleProcessLocal(ctx, r.Config)
	} else {
		logger := logging.NewTestLogger(t, testing.Verbose())
		multiCtx, multiCleanup, err := initMultiProcess(ctx, t.Name(), isBench, r, logger.Log)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cleanup = multiCtx, multiCleanup
	}

	if err := runWeaver(ctx, r, runner); err != nil {
		t.Fatal(err)
	}
}

// checkRunFunc checks that the type of the function passed to
// weavertest.Run is correct (its first argument matches t and its
// remaining arguments are components). On success it returns a
// function that gets the components and passes them to fn.
func checkRunFunc(t testing.TB, fn any) (func(context.Context, any) error, error) {
	fnType := reflect.TypeOf(fn)
	if fnType == nil || fnType.Kind() != reflect.Func {
		return nil, fmt.Errorf("not a func")
	}
	if fnType.IsVariadic() {
		return nil, fmt.Errorf("must not be variadic")
	}
	n := fnType.NumIn()
	if n < 2 {
		return nil, fmt.Errorf("must have at least two args")
	}
	if fnType.NumOut() > 0 {
		return nil, fmt.Errorf("must have no return outputs")
	}

	if fnType.In(0) != reflect.TypeOf(t) {
		return nil, fmt.Errorf("function first argument type %v does not match first weavertest.Run argument %T", fnType.In(0), t)
	}

	return func(ctx context.Context, impl any) error {
		args := make([]reflect.Value, n)
		args[0] = reflect.ValueOf(t)
		for i := 1; i < n; i++ {
			argType := fnType.In(i)
			comp, err := private.Get(impl, argType)
			if err != nil {
				return err
			}
			args[i] = reflect.ValueOf(comp)
		}
		reflect.ValueOf(fn).Call(args)
		return nil
	}, nil
}

func runWeaver(ctx context.Context, runner Runner, body func(context.Context, any) error) error {
	opts := private.RunOptions{Fakes: map[reflect.Type]any{}}
	for _, f := range runner.Fakes {
		opts.Fakes[f.intf] = f.impl
	}
	return private.Run(ctx, reflect.TypeOf((*testMainInterface)(nil)).Elem(), opts, body)
}

// logStacks prints the stacks of live goroutines. This functionality
// is disabled by default but can be enabled to find background work that
// is not obeying cancellation.
func logStacks() {
	time.Sleep(time.Second) // Hack to wait for goroutines to end
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	fmt.Fprintln(os.Stderr, string(buf[:n]))
}
