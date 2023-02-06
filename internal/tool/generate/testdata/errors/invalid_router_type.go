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

// ERROR: invalid routing key type

// Invalid router type.
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type foo interface {
	A(context.Context) error
}

type impl struct {
	weaver.Implements[foo]
	weaver.WithRouter[fooRouter]
}

type fooRouter struct{}

type key struct {
	x int
	y chan int
}

func (fooRouter) A(context.Context) key {
	return key{}
}
