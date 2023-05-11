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
	"sync"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/private"
)

// Options configure weavertest.Init.
type Options struct {
	// If true, every component is colocated in a single process. Otherwise,
	// every component is run in a separate OS process.
	SingleProcess bool

	// Config contains configuration identical to what might be found in a
	// Service Weaver config file. It can contain application level as well as component
	// level configuration. Config is allowed to be empty.
	Config string
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

// Run is a testing version of weaver.Run. Run is passed a function that accepts
// a list of components. Run will create a brand new weaver application execution
// environment, create the components whose types are arguments to testBody,
// and callTestBody with these components.
//
//	func TestFoo(t *testing.T) {
//	    weavertest.Run(t, weavertest.Options{}, func(foo Foo, bar Bar) {
//		// Test foo and bar ...
//	    })
//	}
//
// The test fails at runtime if testBody is not a function whose signature looks like:
//
//	func(ComponentType1)
//	func(ComponentType, ComponentType2)
//	...
func Run(t testing.TB, opts Options, testBody any) {
	_, isBench := t.(*testing.B)
	t.Helper()
	runner, err := checkRunFunc(testBody)
	if err != nil {
		t.Fatal(fmt.Errorf("weavertest.Run argument: %v", err))
	}

	// Make a log writer that forwards to t.
	var logMu sync.Mutex
	logToTest := true
	logWriter := func(entry string) {
		logMu.Lock()
		defer logMu.Unlock()
		if logToTest {
			t.Log(entry)
		} else {
			// Test has exited, so write to stderr so we don't lose
			// useful debugging information.
			fmt.Fprintln(os.Stderr, entry)
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

		// Do not let active goroutines attempt to use t from now on.
		logMu.Lock()
		logToTest = false
		logMu.Unlock()

		// Enable the following to print stacks of goroutine that did not shut down properly.
		if false {
			logStacks()
		}
	}()

	if opts.SingleProcess {
		ctx = initSingleProcess(ctx, opts.Config)
	} else {
		multiCtx, multiCleanup, err := initMultiProcess(ctx, t.Name(), isBench, opts.Config, logWriter)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cleanup = multiCtx, multiCleanup
	}

	if err := runWeaver(ctx, runner); err != nil {
		t.Fatal(err)
	}
}

// checkRunFunc checks that the type of the function passed to weavertest.Run
// is correct (its arguments are components). On success it returns a function
// that gets the components and passes them to fn.
func checkRunFunc(fn any) (func(context.Context, any) error, error) {
	fnType := reflect.TypeOf(fn)
	if fnType == nil || fnType.Kind() != reflect.Func {
		return nil, fmt.Errorf("not a func")
	}
	if fnType.IsVariadic() {
		return nil, fmt.Errorf("must not be variadic")
	}
	n := fnType.NumIn()
	if n == 0 {
		return nil, fmt.Errorf("must have at least one arg")
	}
	if fnType.NumOut() > 0 {
		return nil, fmt.Errorf("must have no return outputs")
	}

	return func(ctx context.Context, impl any) error {
		args := make([]reflect.Value, n)
		for i := range args {
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

func runWeaver(ctx context.Context, body func(context.Context, any) error) error {
	return private.Run(ctx, reflect.TypeOf((*testMainInterface)(nil)).Elem(), body)
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
