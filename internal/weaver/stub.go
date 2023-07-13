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
	"context"

	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"go.opentelemetry.io/otel/trace"
)

// stub holds information about a client stub to the remote component.
type stub struct {
	component string           // name of the remote component
	conn      call.Connection  // connection to talk to the remote component
	methods   []call.MethodKey // keys for the remote component methods
	balancer  call.Balancer    // if not nil, component load balancer
	tracer    trace.Tracer     // component tracer
}

var _ codegen.Stub = &stub{}

// Tracer implements the codegen.Stub interface.
func (s *stub) Tracer() trace.Tracer {
	return s.tracer
}

// Run implements the codegen.Stub interface.
func (s *stub) Run(ctx context.Context, method int, args []byte, shardKey uint64) ([]byte, error) {
	opts := call.CallOptions{
		ShardKey: shardKey,
		Balancer: s.balancer,
	}
	return s.conn.Call(ctx, s.methods[method], args, opts)
}
