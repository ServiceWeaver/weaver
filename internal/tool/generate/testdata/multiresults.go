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
// A(ctx context.Context) (r0 string, r1 int, r2 Bar, err error)
// results, err = s.stub.Run(ctx, 0, nil, shardKey)
// r0 = dec.String()
// r1 = dec.Int()
// func (x *Bar) WeaverMarshal(enc *codegen.Encoder)
// func (x *Bar) WeaverUnmarshal(dec *codegen.Decoder)
// err = s.caller("A", ctx, []any{}, []any{&r0, &r1, &r2})

// UNEXPECTED
// Preallocate

// Multiple results.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type Foo interface {
	A(context.Context) (string, int, Bar, error)
}

type Bar struct {
	weaver.AutoMarshal
}

type impl struct{ weaver.Implements[Foo] }

func (l *impl) A(context.Context) (string, int, Bar, error) {
	return "", 0, Bar{}, nil
}
