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

// EXPECTED: package foo
// EXPECTED: type foo interface {
// EXPECTED: _makeFoo
// EXPECTED: type foo_client_stub struct
// EXPECTED: A(ctx context.Context) (err error)
// EXPECTED: c.Run(0)
// EXPECTED: B(ctx context.Context) (err error)
// EXPECTED: c.Run(1)
// UNEXPECTED: Preallocate

// Multiple methods.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type foo interface {
	A(context.Context) error
	B(context.Context) error
}

type impl struct{ weaver.Implements[foo] }

func (l *impl) A(context.Context) error {
	return nil
}

func (l *impl) B(context.Context) error {
	return nil
}
