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
// EncodeProto
// DecodeProto
// serviceweaver_enc_slice_ptr_TypeProto
// serviceweaver_dec_map_string_ptr_TypeProto

// Proto types.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type TypeProto struct {
	A string
	B int
	C []string
}

func (t *TypeProto) ProtoReflect() protoreflect.Message { return nil }

type foo interface {
	M(context.Context, *TypeProto) (*TypeProto, error)
	N(context.Context, []*TypeProto) (map[string]*TypeProto, error)
}

type impl struct{ weaver.Implements[foo] }

func (l *impl) M(context.Context, *TypeProto) (*TypeProto, error) {
	return nil, nil
}

func (l *impl) N(context.Context, []*TypeProto) (map[string]*TypeProto, error) {
	return nil, nil
}
