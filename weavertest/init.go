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

	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/internal/weaver"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"golang.org/x/exp/slices"
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
// REQUIRES: impl must implement T.
func Fake[T any](impl any) FakeComponent {
	t := reflection.Type[T]()
	if _, ok := impl.(T); !ok {
		panic(fmt.Sprintf("%T does not implement %v", impl, t))
	}
	return FakeComponent{intf: t, impl: impl}
}

// Test runs a sub-test of t that tests the supplied Service Weaver
// application code. It fails at runtime if body is not a function
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
//		weavertest.Local.Test(t, func(t *testing.T, foo Foo, bar *bar) {
//			// Test foo and bar ...
//		})
//	}
//
// Component arguments can either be component interface types (e.g., Foo) or
// component implementation pointer types (e.g., *bar).
//
// In contrast with weaver.Run, the Test method does not run the Main method of
// any registered weaver.Main component.
func (r Runner) Test(t *testing.T, body any) {
	t.Helper()
	t.Run(r.Name, func(t *testing.T) { r.sub(t, false, body) })
}

// Bench runs a sub-benchmark of b that benchmarks the supplied Service
// Weaver application code. It fails at runtime if body is not a
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
//		weavertest.Local.Bench(b, func(b *testing.B, foo Foo, bar *bar) {
//			for i := 0; i < b.N; i++ {
//				// Benchmark foo and bar ...
//			}
//		})
//	}
//
// Component arguments can either be component interface types (e.g., Foo) or
// component implementation pointer types (e.g., *bar).
//
// In contrast with weaver.Run, the Bench method does not run the Main method of
// any registered weaver.Main component.
func (r Runner) Bench(b *testing.B, testBody any) {
	b.Helper()
	b.Run(r.Name, func(b *testing.B) { r.sub(b, true, testBody) })
}

func (r Runner) sub(t testing.TB, isBench bool, testBody any) {
	t.Helper()
	body, intfs, err := checkRunFunc(t, testBody)
	if err != nil {
		t.Fatal(fmt.Errorf("weavertest.Run argument: %v", err))
	}

	// Assume a component Foo implementing struct foo. We disallow tests
	// like the one below where the user provides a fake and a component
	// implementation pointer for the same component.
	//
	//     runner.Fakes = append(runner.Fakes, weavertest.Fake[Foo](...))
	//     runner.Test(t, func(t *testing.T, f *foo) {...})
	for _, intf := range intfs {
		if slices.ContainsFunc(r.Fakes, func(f FakeComponent) bool { return f.intf == intf }) {
			t.Fatalf("Component %v has both fake and component implementation pointer", intf)
		}
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

	fakes := map[reflect.Type]any{}
	for _, f := range r.Fakes {
		fakes[f.intf] = f.impl
	}

	var runner weaver.Weavelet
	if !r.multi && !r.forceRPC {
		opts := weaver.SingleWeaveletOptions{
			Fakes:  fakes,
			Config: r.Config,
			Quiet:  !testing.Verbose(),
		}
		var err error
		runner, err = weaver.NewSingleWeavelet(ctx, codegen.Registered(), opts)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		logger := logging.NewTestLogger(t, testing.Verbose())
		bootstrap, multiCleanup, err := initMultiProcess(ctx, t, isBench, r, intfs, logger.Log)
		if err != nil {
			t.Fatal(err)
		}
		cleanup = multiCleanup

		opts := weaver.RemoteWeaveletOptions{Fakes: fakes}
		runner, err = weaver.NewRemoteWeavelet(ctx, codegen.Registered(), bootstrap, opts)
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := body(ctx, runner); err != nil {
		t.Fatal(err)
	}
}

// checkRunFunc checks that the type of the function passed to weavertest.Run
// is correct (its first argument matches t and its remaining arguments are
// either component interfaces or pointer to component implementations). On
// success it returns (1) a function that gets the components and passes them
// to fn and (2) the interface types of the component implementation arguments.
func checkRunFunc(t testing.TB, fn any) (func(context.Context, weaver.Weavelet) error, []reflect.Type, error) {
	fnType := reflect.TypeOf(fn)
	if fnType == nil || fnType.Kind() != reflect.Func {
		return nil, nil, fmt.Errorf("not a func")
	}
	if fnType.IsVariadic() {
		return nil, nil, fmt.Errorf("must not be variadic")
	}
	n := fnType.NumIn()
	if n < 2 {
		return nil, nil, fmt.Errorf("must have at least two args")
	}
	if fnType.NumOut() > 0 {
		return nil, nil, fmt.Errorf("must have no return outputs")
	}
	if fnType.In(0) != reflect.TypeOf(t) {
		return nil, nil, fmt.Errorf("function first argument type %v does not match first weavertest.Run argument %T", fnType.In(0), t)
	}
	var intfs []reflect.Type
	for i := 1; i < n; i++ {
		switch fnType.In(i).Kind() {
		case reflect.Interface:
			// Do nothing.
		case reflect.Pointer:
			intf, err := extractComponentInterfaceType(fnType.In(i).Elem())
			if err != nil {
				return nil, nil, err
			}
			intfs = append(intfs, intf)
		default:
			return nil, nil, fmt.Errorf("function argument %d type %v must be a component interface or pointer to component implementation", i, fnType.In(i))
		}
	}

	return func(ctx context.Context, runner weaver.Weavelet) error {
		args := make([]reflect.Value, n)
		args[0] = reflect.ValueOf(t)
		for i := 1; i < n; i++ {
			argType := fnType.In(i)
			switch argType.Kind() {
			case reflect.Interface:
				comp, err := runner.GetIntf(argType)
				if err != nil {
					return err
				}
				args[i] = reflect.ValueOf(comp)
			case reflect.Pointer:
				comp, err := runner.GetImpl(argType.Elem())
				if err != nil {
					return err
				}
				args[i] = reflect.ValueOf(comp)
			default:
				return fmt.Errorf("argument %v has unexpected type %v", i, argType)
			}
		}
		reflect.ValueOf(fn).Call(args)
		return nil
	}, intfs, nil
}

// extractComponentInterfaceType extracts the component interface type from the
// provided component implementation. For example, calling
// extractComponentInterfaceType on a struct that embeds weaver.Implements[Foo]
// returns Foo.
//
// extractComponentInterfaceType returns an error if the provided type is not a
// component implementation.
func extractComponentInterfaceType(t reflect.Type) (reflect.Type, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("type %v is not a struct", t)
	}
	// See the definition of weaver.Implements.
	f, ok := t.FieldByName("component_interface_type")
	if !ok {
		return nil, fmt.Errorf("type %v does not embed weaver.Implements", t)
	}
	return f.Type, nil
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
