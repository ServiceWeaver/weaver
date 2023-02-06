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
	"errors"

	"go.opentelemetry.io/otel/trace"
	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

// retriable is a retriable error. If err is a retriable, then errors.Is(err,
// ErrRetriable) is true.
//
// TODO(mwhittaker): Once [1] is implemented, we don't need retriable. We can
// instead join an error with ErrRetriable using fmt.Errorf.
//
// [1]: https://github.com/golang/go/issues/53435
type retriable struct {
	err error
}

// Error implements the error interface.
func (r retriable) Error() string {
	return r.err.Error()
}

// Is makes retriable compatible with errors.Is.
func (r retriable) Is(err error) bool {
	return err == ErrRetriable
}

// Unwrap makes systemError compatible with errors.Is, errors.As, and
// errors.Unwrap.
func (r retriable) Unwrap() error {
	return r.err
}

// stub holds information about a client stub to the remote component.
type stub struct {
	client   call.Connection  // client to talk to the remote component, created lazily.
	methods  []call.MethodKey // Keys for the remote component methods.
	balancer call.Balancer    // if not nil, component load balancer
	tracer   trace.Tracer     // component tracer
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
	return s.client.Call(ctx, s.methods[method], args, opts)
}

// WrapError implements the codegen.Stub interface.
func (s *stub) WrapError(err error) error {
	if errors.Is(err, call.CommunicationError) || errors.Is(err, call.Unreachable) {
		return retriable{err}
	}
	return err
}
