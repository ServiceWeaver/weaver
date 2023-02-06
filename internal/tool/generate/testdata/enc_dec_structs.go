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
// func (x *XYZ) WeaverMarshal(enc *codegen.Encoder)
// func (x *XYZ) WeaverUnmarshal(dec *codegen.Decoder)
// func (x *XY) WeaverMarshal(enc *codegen.Encoder)
// func (x *XY) WeaverUnmarshal(dec *codegen.Decoder)
// func (x *X) WeaverMarshal(enc *codegen.Encoder)
// func (x *X) WeaverUnmarshal(dec *codegen.Decoder)
// func (x *Z) WeaverMarshal(enc *codegen.Encoder)
// func (x *Z) WeaverUnmarshal(dec *codegen.Decoder)
// func (x *XYZ) WeaverMarshal(enc *codegen.Encoder)
// func (x *XYZ) WeaverUnmarshal(dec *codegen.Decoder)
// func (x *XY) WeaverMarshal(enc *codegen.Encoder)
// func (x *XY) WeaverUnmarshal(dec *codegen.Decoder)
// func (x *X) WeaverMarshal(enc *codegen.Encoder)
// func (x *X) WeaverUnmarshal(dec *codegen.Decoder)
// func (x *Z) WeaverMarshal(enc *codegen.Encoder)
// func (x *Z) WeaverUnmarshal(dec *codegen.Decoder)
// func (x *Y) WeaverMarshal(enc *codegen.Encoder)
// func (x *Y) WeaverUnmarshal(dec *codegen.Decoder)
// func (x *Y) WeaverMarshal(enc *codegen.Encoder)
// func (x *Y) WeaverUnmarshal(dec *codegen.Decoder)
// func (x *W) WeaverMarshal(enc *codegen.Encoder)
// func (x *W) WeaverUnmarshal(dec *codegen.Decoder)
// func (x *W) WeaverMarshal(enc *codegen.Encoder)
// func (x *W) WeaverUnmarshal(dec *codegen.Decoder)
// EncodeBinaryMarshaler
// DecodeBinaryUnmarshaler

// UNEXPECTED
// Preallocate

// Generate methods for nested structs. Verify that for structs that have
// all types in the same package or that have custom (Un)marshalBinary methods,
// enc/dec methods are generated.
package foo

import (
	"context"
	"time"

	weaver "github.com/ServiceWeaver/weaver"
)

type foo interface {
	M(ctx context.Context, x X, y Y, z Z, w W) error
}

type impl struct{ weaver.Implements[foo] }

func (l *impl) M(ctx context.Context, x X, y Y, z Z, w W) error {
	return nil
}

type X struct {
	weaver.AutoMarshal
	A1 XY
}

type XY struct {
	weaver.AutoMarshal
	A1 XYZ
	B1 string
}

type XYZ struct {
	weaver.AutoMarshal
	A1 int64
	A2 string
}

type Y struct {
	weaver.AutoMarshal
	ID    int64
	Label string
	When  time.Time
	Text  string
}

type Z struct {
	weaver.AutoMarshal
	id string
}

type W struct {
	weaver.AutoMarshal
	id time.Time
}
