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
// func (x *A) WeaverMarshal(enc *codegen.Encoder)
// func (x *A) WeaverUnmarshal(dec *codegen.Decoder)
// func (x *B) WeaverMarshal(enc *codegen.Encoder)
// func (x *B) WeaverUnmarshal(dec *codegen.Decoder)

// Verify that AutoMarshal works on a struct with an embedded struct that also
// embeds AutoMarshal.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type A struct {
	weaver.AutoMarshal
	B
}

type B struct {
	weaver.AutoMarshal
}

type foo interface {
	M(context.Context, A, B) error
}

type impl struct{ weaver.Implements[foo] }
