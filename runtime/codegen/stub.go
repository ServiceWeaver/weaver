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

package codegen

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// A Stub allows a Service Weaver component in one process to invoke methods via RPC on
// a Service Weaver component in a different process.
type Stub interface {
	// Tracer returns a new tracer.
	//
	// TODO(mwhittaker): Move tracer out of stub? It doesn't really fit with
	// the abstraction?
	Tracer() trace.Tracer

	// Run executes the provided method with the provided serialized arguments.
	// At code generation time, an object's methods are deterministically
	// ordered. method is the index into this slice. args and results are the
	// serialized arguments and results, respectively. shardKey is the shard
	// key for routed components, and 0 otherwise.
	Run(ctx context.Context, method int, args []byte, shardKey uint64) (results []byte, err error)

	// WrapError embeds ErrRetriable into the appropriate errors. The codegen
	// package cannot perform this wrapping itself because of cyclic
	// dependencies.
	WrapError(error) error
}

// A Server allows a Service Weaver component in one process to receive and execute
// methods via RPC from a Service Weaver component in a different process. It is the
// dual of a Stub.
type Server interface {
	// GetStubFn returns a handler function for the given method. For example,
	// if a Service Weaver component defined an Echo method, then GetStubFn("Echo")
	// would return a handler that deserializes the arguments, executes the
	// method, and serializes the results.
	//
	// TODO(mwhittaker): Rename GetHandler? This is returning a call.Handler.
	GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error)
}
