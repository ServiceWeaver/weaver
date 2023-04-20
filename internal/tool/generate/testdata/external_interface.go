// Copyright 2023 Google LLC
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
// reflect.TypeOf((*pkg.Adder)(nil)).Elem(),
// func() any { return &impl{} },
// func(i any) any { return i.(*impl).WithConfig.Config() },
// return impl_local_stub{impl: impl.(pkg.Adder), tracer: tracer}
// return impl_client_stub
// return impl_server_stub{impl: impl.(pkg.Adder), addLoad: addLoad}
// _hashImpl
// _orderedCodeImpl

// A component that implements an interface in a different package.
package foo

import (
	"context"

	sub1 "foo/sub1"

	"github.com/ServiceWeaver/weaver"
)

type config struct{}

type impl struct {
	weaver.Implements[sub1.Adder]
	weaver.WithConfig[config]
	weaver.WithRouter[router]
}

func (*impl) Add(_ context.Context, x, y int) (int, error) {
	return x + y, nil
}

type router struct{}

func (*router) Add(_ context.Context, x, y int) int {
	return x + y
}
