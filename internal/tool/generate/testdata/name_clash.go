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
// "foo/sub1"
// pkg1 "foo/sub2"
// A(ctx context.Context, a0 pkg.T, a1 pkg1.T)

// UNEXPECTED
// Preallocate

// Packages with identical default entity.
package foo

import (
	"context"

	sub1 "foo/sub1"
	sub2 "foo/sub2"

	"github.com/ServiceWeaver/weaver"
)

type foo interface {
	A(context.Context, sub1.T, sub2.T) error
}

type impl struct{ weaver.Implements[foo] }

func (l *impl) A(context.Context, sub1.T, sub2.T) error {
	return nil
}
