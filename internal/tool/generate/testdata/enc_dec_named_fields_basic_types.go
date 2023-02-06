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
// enc.Int((int)(a0))
// enc.String((string)(a1))
// *(*int)(&a0) = dec.Int()
// *(*string)(&a1) = dec.String()
// Preallocate

// UNEXPECTED
// func serviceweaver_enc_A
// func serviceweaver_dec_A
// func serviceweaver_enc_B
// func serviceweaver_dec_B
// serviceweaver_size_A(x *A)
// serviceweaver_size_B(x *B)

// Generate methods for named types that are basic types. Verify that no
// enc/dec methods are generated for the types, and we rely on basic types
// enc/dec instead.
package foo

import (
	"context"

	weaver "github.com/ServiceWeaver/weaver"
)

type A int
type B string
type Foo interface{}

type foo interface {
	M(context.Context, A, B) error
}

type impl struct{ weaver.Implements[foo] }

func (l *impl) M(context.Context, A, B) error {
	return nil
}
