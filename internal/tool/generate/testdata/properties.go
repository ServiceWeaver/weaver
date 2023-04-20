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
// codegen.Register(codegen.Registration{
// Routed:
// true,
// _hashImpl(r.M(ctx))
// func _hashImpl(r int) uint64
// func _orderedCodeImpl(r int)

// UNEXPECTED
// Preallocate

// Component properties.
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
	weaver.WithRouter[fooRouter]
}

func (l *impl) M(context.Context) error {
	return nil
}

type fooRouter struct{}

func (fooRouter) M(context.Context) int { return 0 }
