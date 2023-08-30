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
// NoRetry: []int{1, 3}

// Package foo contains some a component with some non-retriable methods.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type foo interface {
	A(context.Context) error
	B(context.Context) error
	C(context.Context) error
	D(context.Context) error
}

type impl struct{ weaver.Implements[foo] }

func (l *impl) A(context.Context) error { return nil }
func (l *impl) B(context.Context) error { return nil }
func (l *impl) C(context.Context) error { return nil }
func (l *impl) D(context.Context) error { return nil }

var _ weaver.NotRetriable = foo.B
var _ weaver.NotRetriable = foo.D

// Following should be ignored.
var _ any = foo.A
