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
// addLoad
// Routed:
// true,
// func _hashImpl(r fooKey) uint64
// func _orderedCodeImpl(r fooKey) codegen.OrderedCode

// Service Weaver route keys.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type foo interface {
	M(context.Context, int) error
}

type impl struct {
	weaver.Implements[foo]
	weaver.WithRouter[fooRouter]
}

func (l *impl) M(context.Context, int) error {
	return nil
}

type fooRouter struct{}

func (fooRouter) M(_ context.Context, i int) fooKey { return fooKey{i, "hello"} }

type fooKey struct {
	x int
	y string
}
