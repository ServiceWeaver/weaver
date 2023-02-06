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
// serviceweaver_size_ptr_ptr_ptr_A
// serviceweaver_size_ptr_ptr_A
// serviceweaver_size_ptr_A
// serviceweaver_size_A
// serviceweaver_size_ptr_ptr_ptr_B
// serviceweaver_size_ptr_ptr_B
// serviceweaver_size_ptr_B
// serviceweaver_size_B
// serviceweaver_size_ptr_ptr_ptr_int
// serviceweaver_size_ptr_ptr_int
// serviceweaver_size_ptr_int

// Deeply nested pointers and structs.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type A struct {
	weaver.AutoMarshal
	b ***B
}

type B struct {
	weaver.AutoMarshal
	x ***int
}

type foo interface {
	M(context.Context, ***A) error
}

type impl struct{ weaver.Implements[foo] }

func (l *impl) M(context.Context, ***A) error {
	return nil
}
