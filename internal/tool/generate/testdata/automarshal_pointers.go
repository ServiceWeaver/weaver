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
// serviceweaver_enc_ptr_t
// serviceweaver_dec_ptr_t
// func (x *t) WeaverMarshal(enc *codegen.Encoder)
// func (x *t) WeaverUnmarshal(dec *codegen.Decoder)

// Verify that AutoMarshal structs can be received by value and by pointer.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type t struct {
	weaver.AutoMarshal
}

type foo interface {
	ByValue(context.Context, t) (t, error)
	ByPointer(context.Context, *t) (*t, error)
}

type impl struct{ weaver.Implements[foo] }
