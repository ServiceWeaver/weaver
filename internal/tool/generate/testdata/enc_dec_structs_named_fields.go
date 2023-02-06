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
// func (x *X) WeaverMarshal(enc *codegen.Encoder)
// func (x *X) WeaverUnmarshal(dec *codegen.Decoder)
// enc.Int(x.A1)
// enc.String(x.A2)
// enc.Int((int)(x.A3))
// enc.String((string)(x.A4))
// dec.Int()
// dec.String()
// *(*int)(&x.A3) = dec.Int()
// *(*string)(&x.A4) = dec.String()

// UNEXPECTED
// func serviceweaver_enc_ItemID
// func serviceweaver_dec_ItemID
// func serviceweaver_enc_ConfigID
// func serviceweaver_dec_ConfigID
// Preallocate

// Generate methods for structs with named types. Verify that for structs
// that have named fields that point to various types, enc/dec methods are
// generated.
package foo

import (
	"context"

	weaver "github.com/ServiceWeaver/weaver"
)

type foo interface {
	M(context.Context, X, Y) error
}

type impl struct{ weaver.Implements[foo] }

func (l *impl) M(ctx context.Context, x X, y Y) error {
	return nil
}

type ItemID int
type ConfigID string

type X struct {
	weaver.AutoMarshal
	A1 int
	A2 string
	A3 ItemID
	A4 ConfigID
}

type Y map[ConfigID][]X
