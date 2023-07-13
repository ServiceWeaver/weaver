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
	"crypto/tls"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/register"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
)

// componentImpl is a fully instantiated Service Weaver component that is running locally on
// this process. If the component's method is invoked from the local process,
// a local stub is used; otherwise, a server stub is used.
type componentImpl struct {
	component  *component     // passed to component constructor
	impl       interface{}    // user implementation of component
	serverStub codegen.Server // handles calls from other processes
}

// component represents a Service Weaver component and all corresponding metadata.
type component struct {
	wlet      *Weavelet             // read-only, once initialized
	info      *codegen.Registration // read-only, once initialized
	clientTLS *tls.Config           // read-only, once initialized

	registerInit sync.Once // used to register the component
	registerErr  error     // non-nil if registration fails

	implInit sync.Once      // used to initialize impl, logger
	implErr  error          // non-nil if impl creation fails
	impl     *componentImpl // only ever non-nil if this component is local
	logger   *slog.Logger   // read-only after implInit.Do()
	tracer   trace.Tracer   // read-only after implInit.Do()

	// TODO(mwhittaker): We have one client for every component. Every client
	// independently maintains network connections to every weavelet hosting
	// the component. Thus, there may be many redundant network connections to
	// the same weavelet. Given n weavelets hosting m components, there's at
	// worst n^2m connections rather than a more optimal n^2 (a single
	// connection between every pair of weavelets). We should rewrite things to
	// avoid the redundancy.
	clientInit sync.Once // used to initialize client
	client     *client   // only evern non-nil if this component is remote or routed

	stubInit sync.Once // used to initialize stub
	stubErr  error     // non-nil if stub creation fails
	stub     *stub     // only ever non-nil if this component is remote or routed

	local register.WriteOnce[bool] // routed locally?
	load  *loadCollector           // non-nil for routed components
}
