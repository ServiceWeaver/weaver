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
// var a0 map[int][]X
// var a1 map[int]bool
// var a2 map[[10]int]int
// r0, appErr := s.impl.A
// serviceweaver_enc_map_int_slice_X
// serviceweaver_enc_map_int_bool
// serviceweaver_dec_map_array_10_int_int
// serviceweaver_enc_map_Y_map_string_slice_X

// UNEXPECTED
// Preallocate
// c.Args.Encode
// c.Results.Decode

// Methods with maps as arguments and results.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type Y struct {
	weaver.AutoMarshal
	a int
	b X
}

type X struct {
	weaver.AutoMarshal
	a int
}

type foo interface {
	A(context.Context, map[int][]X, map[int]bool, map[[10]int]int) (map[Y]map[string][]X, error)
}

type impl struct{ weaver.Implements[foo] }

func (l *impl) A(context.Context, map[int][]X, map[int]bool, map[[10]int]int) (map[Y]map[string][]X, error) {
	return nil, nil
}
