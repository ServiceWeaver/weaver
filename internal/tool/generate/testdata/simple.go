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

// EXPECTED
// package foo
// func init() {
// codegen.Register(codegen.Registration{
// impl{}
// return foo_local_stub{impl: impl.(foo),
// return foo_client_stub{stub: stub,
// type foo_local_stub struct
// type foo_client_stub struct
// M(ctx context.Context) (err error) {
// s.stub.Run(ctx, 0, nil, shardKey)
// type foo_server_stub struct
// func (s foo_server_stub) GetStubFn
// func (s foo_server_stub) m(ctx
// type foo_reflect_stub struct

// UNEXPECTED
// Preallocate
// Routed:

// Simple method with no arguments and results.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type foo interface {
	M(context.Context) error
}

type impl struct {
	weaver.Implements[foo]
}

func (l *impl) M(context.Context) error {
	return nil
}
