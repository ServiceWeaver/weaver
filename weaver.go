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
	"reflect"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/private"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

//go:generate ./dev/protoc.sh internal/status/status.proto
//go:generate ./dev/protoc.sh internal/tool/ssh/impl/ssh.proto
//go:generate ./dev/protoc.sh runtime/protos/runtime.proto
//go:generate ./dev/protoc.sh runtime/protos/config.proto
//go:generate ./dev/writedeps.sh

var (
	// RemoteCallError indicates that a remote component method call failed to
	// execute properly. This can happen, for example, because of a failed
	// machine or a network partition. Here's an illustrative example:
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
	// RemoteCallError, it does NOT mean that the method never executed. The
	// method may have executed partially or fully. Thus, you must be careful
	// retrying method calls that result in a RemoteCallError. Ensuring that all
	// methods are either read-only or idempotent is one way to ensure safe
	// retries, for example.
	RemoteCallError = errors.New("Service Weaver remote call error")

	// HealthzHandler is a health-check handler that returns an OK status for
	// all incoming HTTP requests.
	HealthzHandler = func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "OK")
	}
)

const (
	// HealthzURL is the URL path on which Service Weaver performs health
	// checks. Every application HTTP server must register a handler for this
	// URL path, e.g.:
	//
	//   mux := http.NewServeMux()
	//   mux.HandleFunc(weaver.HealthzURL, func(http.ResponseWriter, *http.Request) {
	//	   ...
	//   })
	//
	// As a convenience, Service Weaver registers HealthzHandler under
	// this URL path in the default ServerMux, i.e.:
	//
	//  http.HandleFunc(weaver.HealthzURL, weaver.HealthzHandler)
	HealthzURL = "/debug/weaver/healthz"
)

var healthzInit sync.Once

// Run runs app as a Service Weaver application.
//
// The application is composed of a set of components that include
// weaver.Main as well as any components transitively needed by
// weaver.Main. An instance that implement weaver.Main is
// automatically created by weaver.Run and passed to app.  Note: other
// replicas in which weaver.Run is called may also create instances of
// weaver.Main.
//
// The type T must be a struct type that contains an embedded
// `weaver.Implements[weaver.Main]` field. A value of type T is
// created, initialized (by calling its Init method if any), and a
// pointer to the value is passed to app. app contains the main body of
// the application; it will typically fetch any other components that
// are needed, run HTTP servers, etc.
//
// If this process is hosting the `weaver.Main` component, Run will
// call app and will return when app returns. If this process is
// hosting other components, Run will start those components and never
// return. Most callers of Run will not do anything (other than
// possibly logging any returned error) after Run returns.
//
//	func main() {
//	    if err := weaver.Run(context.Background(), app); err != nil {
//	        log.Fatal(err)
//	    }
//	}
func Run[T InstanceOf[Main]](ctx context.Context, app func(context.Context, T) error) error {
	// Register HealthzHandler in the default ServerMux.
	healthzInit.Do(func() {
		http.HandleFunc(HealthzURL, HealthzHandler)
	})

	rootType := reflect.TypeOf((*Main)(nil)).Elem()
	rootBody := func(ctx context.Context, impl any) error {
		arg, ok := impl.(T)
		if !ok {
			var zero T
			panic(fmt.Sprintf("internal error: object created for main component has type %T instead of expected type %T", impl, zero))
		}
		return app(ctx, arg)
	}
	return internalRun(ctx, rootType, rootBody)
}

func internalRun(ctx context.Context, rootType reflect.Type, rootBody func(context.Context, any) error) error {
	wlet, err := newWeavelet(ctx, codegen.Registered())
	if err != nil {
		return fmt.Errorf("error initializating application: %w", err)
	}
	return wlet.start(rootType, rootBody)
}

func internalGet(requester any, compType reflect.Type) (any, error) {
	rep := requester.(Instance).rep()
	component, err := rep.wlet.getComponentByType(compType)
	if err != nil {
		return nil, err
	}
	result, _, err := rep.wlet.getInstance(component, rep.info.Name)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func init() {
	// Provide weavertest with access to Run and Get.
	private.Run = internalRun
	private.Get = internalGet
}
