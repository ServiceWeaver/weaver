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

// ERROR: "A" does not match any method of "foo"

// Extra routing function.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type foo interface {
	Foo(context.Context) error
}

type impl struct {
	weaver.Implements[foo]
	weaver.WithRouter[fooRouter]
}

func (impl) Foo(context.Context) error {
	return nil
}

type fooRouter struct{}

func (fooRouter) A(context.Context, bool, int, string) int { return 0 }
