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

package diverge

import (
	"context"
	"errors"
	"testing"

	"github.com/ServiceWeaver/weaver/weavertest"
)

//go:generate ../../../cmd/weaver/weaver generate ./...

// TestDealiasing demonstrates that pointers are only de-aliased when we use RPCs.
func TestDealiasing(t *testing.T) {
	for _, runner := range weavertest.AllRunners() {
		runner.Test(t, func(t *testing.T, p Pointer) {
			pair, err := p.Get(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			localCalls := runner == weavertest.Local
			if want, got := localCalls, (pair.X == pair.Y); want != got {
				t.Fatalf("expecting aliasing = %v, got %v", want, got)
			}
		})
	}
}

// TestCustomErrors* demonstrates that custom Is methods are ignored when using RPCs.
func TestCustomErrors(t *testing.T) {
	for _, runner := range weavertest.AllRunners() {
		runner.Test(t, func(t *testing.T, e Errer) {
			err := e.Err(context.Background(), 1)
			localCalls := runner == weavertest.Local
			if want, got := localCalls, errors.Is(err, IntError{2}); want != got {
				t.Fatalf("expecting Is(IntError{2}) = %v, got %v for error %v", want, got, err)
			}
		})
	}
}
