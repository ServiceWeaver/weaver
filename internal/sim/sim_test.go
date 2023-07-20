// Copyright 2023 Google LLC
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

package sim

import (
	"context"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/reflection"
)

func TestSim(t *testing.T) {
	sim, err := New(Options{NumReplicas: 2})
	if err != nil {
		t.Fatal(err)
	}
	x, err := sim.GetIntf(reflection.Type[swapper]())
	if err != nil {
		t.Fatal(err)
	}
	swapper := x.(swapper)
	x, y, err := swapper.Swap(context.Background(), 1, 2)
	if x != 2 {
		t.Errorf("x: got %d, want 2", x)
	}
	if y != 1 {
		t.Errorf("y: got %d, want 1", y)
	}
	if err != (swapperError{}) {
		t.Errorf("err: got %v, want %v", err, swapperError{})
	}
}
