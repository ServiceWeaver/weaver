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

// ERROR: Duplicate implementation for component foo/Adder, other declaration:
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type Adder interface {
	Add(context.Context, int, int) (int, error)
}

type first struct {
	weaver.Implements[Adder]
}

func (f *first) Add(_ context.Context, x, y int) (int, error) {
	return x + y, nil
}

type second struct {
	weaver.Implements[Adder]
}

func (s *second) Add(_ context.Context, x, y int) (int, error) {
	return x + y, nil
}
