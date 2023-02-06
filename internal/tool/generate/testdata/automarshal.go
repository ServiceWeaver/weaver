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

package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

type byValue struct{ notSerializable chan int }
type byPointer struct{ notSerializable chan int }
type mixed1 struct{ notSerializable chan int }
type mixed2 struct{ notSerializable chan int }

func (byValue) WeaverMarshal(enc *codegen.Encoder)      {}
func (byValue) WeaverUnmarshal(dec *codegen.Decoder)    {}
func (*byPointer) WeaverMarshal(enc *codegen.Encoder)   {}
func (*byPointer) WeaverUnmarshal(dec *codegen.Decoder) {}
func (mixed1) WeaverMarshal(enc *codegen.Encoder)       {}
func (*mixed1) WeaverUnmarshal(dec *codegen.Decoder)    {}
func (*mixed2) WeaverMarshal(enc *codegen.Encoder)      {}
func (mixed2) WeaverUnmarshal(dec *codegen.Decoder)     {}

type foo interface {
	M(context.Context, byValue, byPointer, mixed1, mixed2) error
}

type impl struct{ weaver.Implements[foo] }
