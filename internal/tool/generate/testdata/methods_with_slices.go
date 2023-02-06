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
// Preallocate
// serviceweaver_enc_slice_X
// serviceweaver_enc_slice_int
// serviceweaver_enc_slice_string
// serviceweaver_dec_slice_X
// serviceweaver_dec_slice_int
// serviceweaver_dec_slice_string
// serviceweaver_enc_slice_map_int_string
// serviceweaver_enc_slice_slice_X
// serviceweaver_enc_ptr_string
// serviceweaver_dec_ptr_string
// serviceweaver_enc_ptr_X
// serviceweaver_dec_ptr_X
// serviceweaver_size_ptr_string_3e89801b(x *string)
// serviceweaver_size_ptr_X_73ddf179(x *X)
// serviceweaver_size_X_4cd17e8a(x *X)

// UNEXPECTED
// c.Args.Encode(a0)
// c.Args.Encode(a1)
// c.Args.Encode(a2)
// c.Args.Encode(a3)

// Methods with slices as arguments and results.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type X struct {
	weaver.AutoMarshal
	a int
}

type foo interface {
	A(context.Context, []X, []int, [][]X, []map[int]string) ([]string, error)
	B(context.Context, *string, *X) error
}

type impl struct{ weaver.Implements[foo] }

func (l *impl) A(context.Context, []X, []int, [][]X, []map[int]string) ([]string, error) {
	return nil, nil
}

func (l *impl) B(context.Context, *string, *X) error {
	return nil
}
