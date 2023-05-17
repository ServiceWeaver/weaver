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
	name     string
	config   string
}

var (
	// Local is a Runner that places all components in the same process
	// and uses local procedure calls for method invocations.
	Local = Runner{name: "Local"}

	// RPC is a Runner that places all components in the same process
	// and uses RPCs for method invocations.
	RPC = Runner{multi: false, forceRPC: true, name: "RPC"}

	// Multi is a Runner that places all components in different
	// process (unless explicitly colocated) and uses RPCs for method
	// invocations on remote components and local procedure calls for
	// method invocations on colocated components.
	Multi = Runner{multi: true, name: "Multi"}
)

// AllRunners returns a slice of all builtin weavertest runners.
func AllRunners() []Runner { return []Runner{Local, RPC, Multi} }

// WithName returns a new Runner with the specified name. It is useful
// when the runner has been adjusted and is no longer identical to one
// of the predefined runners.
func (r Runner) WithName(name string) Runner { r.name = name; return r }

// WithConfig returns a new Runner with the specified Service Weaver
// configuration. The config value passed here is identical to what
// might be found in a Service Weaver config file. It can contain
// application level as well as component level configuration.
func (r Runner) WithConfig(config string) Runner { r.config = config; return r }

// Name returns the runner name. It is suitable for use as a sub-test or sub-benchmark name.
func (r Runner) Name() string {
	if r.name == "" {
		return "Default"
	}
	return r.name
}

// SingleProcess returns true iff the runner will run all components in a single process.
func (r Runner) SingleProcess() bool { return !r.multi }

// UsesRPC returns true iff the runner uses RPCs for communication.
func (r Runner) UsesRPC() bool { return r.multi || r.forceRPC }

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
//	    weavertest.Local.Run(t, func(foo Foo, bar Bar) {
//		// Test foo and bar ...
//	    })
//	}
//
// The test fails at runtime if testBody is not a function whose signature looks like:
//
//	func(ComponentType1)
//	func(ComponentType, ComponentType2)
//	...
func (r Runner) Run(t testing.TB, testBody any) {
	_, isBench := t.(*testing.B)
	t.Helper()
	runner, err := checkRunFunc(testBody)
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
		ctx = initSingleProcess(ctx, r.config)
	} else {
		logger := logging.NewTestLogger(t, testing.Verbose())
		multiCtx, multiCleanup, err := initMultiProcess(ctx, t.Name(), isBench, r, logger.Log)
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
