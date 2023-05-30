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
// automatically created by weaver.Run and its Main method called.
// Note: other replicas in which weaver.Run is called may also create
// instances of weaver.Main.
//
// If this process is hosting the `weaver.Main` component, Run will
// call its Main method and will return when that method returns. If
// this process is not hosting `weaver.Main`, it will never
// return. Most callers of Run will not do anything (other than
// possibly logging any returned error) after Run returns.
//
//	func main() {
//	    if err := weaver.Run(context.Background()); err != nil {
//	        log.Fatal(err)
//	    }
//	}
func Run(ctx context.Context) error {
	// Register HealthzHandler in the default ServerMux.
	healthzInit.Do(func() {
		http.HandleFunc(HealthzURL, HealthzHandler)
	})

	app, err := internalStart(ctx, private.AppOptions{})
	if err != nil {
		return err
	}
	return app.Wait(ctx)
}

func internalStart(ctx context.Context, opts private.AppOptions) (private.App, error) {
	w, err := newWeavelet(ctx, opts, codegen.Registered())
	if err != nil {
		return nil, fmt.Errorf("error initializating application: %w", err)
	}
	if err := w.start(); err != nil {
		return nil, err
	}
	return w, nil
}

func init() {
	// Provide weavertest with access to weavelet.
	private.Start = internalStart
}
