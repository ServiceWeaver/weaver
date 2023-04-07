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

// Package weaver provides the interface for building
// [single-image distributed programs].
//
// A program is composed of a set of Go interfaces called
// components. Components are recognized by "weaver generate" (typically invoked
// via "go generate"). "weaver generate" generates code that allows a component
// to be invoked over the network. This flexibility allows Service Weaver
// to decompose the program execution across many processes and machines.
//
// [single-image distributed programs]: https://serviceweaver.dev
package weaver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"go.opentelemetry.io/otel/trace"
)

//go:generate ./dev/protoc.sh internal/status/status.proto
//go:generate ./dev/protoc.sh internal/tool/ssh/impl/ssh.proto
//go:generate ./dev/protoc.sh runtime/protos/runtime.proto
//go:generate ./dev/protoc.sh runtime/protos/config.proto
//go:generate ./dev/writedeps.sh

// RemoteCallError indicates that a remote component method call failed to
// execute properly. This can happen, for example, because of a failed machine
// or a network partition. Here's an illustrative example:
//
//	// Call the foo.Foo method.
//	err := foo.Foo(ctx)
//	if errors.Is(err, weaver.RemoteCallError) {
//	    // foo.Foo did not execute properly.
//	} else if err != nil {
//	    // foo.Foo executed properly, but returned an error.
//	} else {
//	    // foo.Foo executed properly and did not return an error.
//	}
//
// Note that if a method call returns an error with an embedded
// RemoteCallError, it does NOT mean that the method never executed. The method
// may have executed partially or fully. Thus, you must be careful retrying
// method calls that result in a RemoteCallError. Ensuring that all methods are
// either read-only or idempotent is one way to ensure safe retries, for
// example.
var RemoteCallError = errors.New("Service Weaver remote call error")

// mainIface is an empty interface "implemented" by the user main function,
// allowing us to treat the user main as a regular Service Weaver component in the
// implementation.
type mainIface interface{}

// mainImpl is the empty implementation of the mainIface "component".
type mainImpl struct {
	Implements[mainIface]
}

func init() {
	// Register the "main" component.
	codegen.Register(codegen.Registration{
		Name:         "main",
		Iface:        reflect.TypeOf((*mainIface)(nil)).Elem(),
		New:          func() any { return &mainImpl{} },
		LocalStubFn:  func(any, trace.Tracer) any { return nil },
		ClientStubFn: func(codegen.Stub, string) any { return nil },
		ServerStubFn: func(any, func(uint64, float64)) codegen.Server { return nil },
	})

	// Add a trivial /healthz handler to the default mux.
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok")) //nolint:errcheck // response write error
	})
}

// Init initializes the execution of a process involved in a Service Weaver application.
//
// Components in a Service Weaver application are executed in a set of processes, potentially
// spread across many machines. Each process executes the same binary and must
// call [weaver.Init]. If this process is hosting the "main" component, Init will return
// a handle to the main component implementation for this process.
//
// If this process is not hosting the "main" component, Init will never return and will
// just serve requests directed at the components being hosted inside the process.
func Init(ctx context.Context) Instance {
	root, err := initInternal(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("error initializing Service Weaver: %w", err))
		os.Exit(1)
	}
	return root
}

func initInternal(ctx context.Context) (Instance, error) {
	wlet, err := newWeavelet(ctx, codegen.Registered())
	if err != nil {
		return nil, fmt.Errorf("internal error creating weavelet: %w", err)
	}

	return wlet.start()
}

// Get returns the distributed component of type T, creating it if necessary.
// The actual implementation may be local, or in another process, or perhaps
// even replicated across many processes. requester represents the component
// that is fetching the component of type T. For example:
//
//	func main() {
//	    root := weaver.Init(context.Background())
//	    foo := weaver.Get[Foo](root) // Get the Foo component.
//	    // ...
//	}
//
// Components are constructed the first time you call Get. Constructing a
// component can sometimes be expensive. When deploying a Service Weaver application on
// the cloud, for example, constructing a component may involve launching a
// container. For this reason, we recommend you call Get proactively to incur
// this overhead at initialization time rather than on the critical path of
// serving a client request.
func Get[T any](requester Instance) (T, error) {
	var zero T
	iface := reflect.TypeOf(&zero).Elem()
	rep := requester.rep()
	component, err := rep.wlet.getComponentByType(iface)
	if err != nil {
		return zero, err
	}
	result, err := rep.wlet.getInstance(component, rep.info.Name)
	if err != nil {
		return zero, err
	}
	return result.(T), nil
}
