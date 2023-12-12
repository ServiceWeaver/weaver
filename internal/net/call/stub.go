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

package call

import (
	"context"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"go.opentelemetry.io/otel/trace"
)

// stub holds information about a client stub to the remote component.
type stub struct {
	conn          Connection   // connection to talk to the remote component
	methods       []stubMethod // per method info
	tracer        trace.Tracer // component tracer
	injectRetries int          // Number of artificial retries per retriable call
}

type stubMethod struct {
	key   MethodKey // key for remote component method
	retry bool      // Whether or not the method should be retred
}

var _ codegen.Stub = &stub{}

// NewStub creates a client-side stub of the type matching reg. Calls on the stub are sent on
// conn to the component with the specified name.
func NewStub(name string, reg *codegen.Registration, conn Connection, tracer trace.Tracer, injectRetries int) codegen.Stub {
	return &stub{
		conn:          conn,
		methods:       makeStubMethods(name, reg),
		tracer:        tracer,
		injectRetries: injectRetries,
	}
}

// Tracer implements the codegen.Stub interface.
func (s *stub) Tracer() trace.Tracer {
	return s.tracer
}

// Run implements the codegen.Stub interface.
func (s *stub) Run(ctx context.Context, method int, args []byte, shardKey uint64) (result []byte, err error) {
	m := s.methods[method]
	opts := CallOptions{
		Retry:    m.retry,
		ShardKey: shardKey,
	}
	n := 1
	if m.retry {
		n += s.injectRetries
	}
	for i := 0; i < n; i++ {
		result, err = s.conn.Call(ctx, m.key, args, opts)
		// No backoff since these retries are fake ones injected for testing.
	}
	return
}

// makeStubMethods returns a slice of stub methods for the component methods of reg.
func makeStubMethods(fullName string, reg *codegen.Registration) []stubMethod {
	// Construct method info slice.
	n := reg.Iface.NumMethod()
	methods := make([]stubMethod, n)
	for i := 0; i < n; i++ {
		mname := reg.Iface.Method(i).Name
		methods[i].key = MakeMethodKey(fullName, mname)
		methods[i].retry = true // Retry by default
	}
	for _, m := range reg.NoRetry {
		methods[m].retry = false
	}
	return methods
}
