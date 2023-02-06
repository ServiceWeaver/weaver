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
// type foo_client_stub struct
// type foo_server_stub struct
// A(ctx context.Context, a0 string, a1 int, a2 Bar, a3 Other) (err error)
// s.stub.Run(ctx, 0, enc.Data(), shardKey)
// enc.String(a0)
// enc.Int(a1)
// func (x *Bar) WeaverMarshal(enc *codegen.Encoder)
// func (x *Bar) WeaverUnmarshal(dec *codegen.Decoder)
// EncodeBinaryMarshaler
// &impl{}

// UNEXPECTED
// c.Args.Encode(a3)
// Preallocate

// Multiple args.
package foo

import (
	"context"
	"time"

	weaver "github.com/ServiceWeaver/weaver"
)

type Bar struct {
	weaver.AutoMarshal
}

type Other struct {
	weaver.AutoMarshal
	T time.Time
}

type impl struct{ weaver.Implements[Foo] }

type Foo interface {
	A(context.Context, string, int, Bar, Other) error
}

func (l *impl) A(context.Context, string, int, Bar, Other) error {
	return nil
}
