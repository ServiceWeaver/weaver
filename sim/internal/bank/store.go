// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bank

import (
	"context"
	"fmt"

	"github.com/ServiceWeaver/weaver"
)

// A Store is a persistent map from strings to integers, like a map[string]int.
type Store interface {
	// Get gets the value of the provided key.
	Get(ctx context.Context, key string) (int, error)

	// Add atomically adds the provided delta to the provided key. Note that
	// delta can be positive or negative. For example, Add(ctx, "foo", 10) adds
	// 10 to "foo", while Add(ctx, "foo", -10) subtracts 10 from "foo".
	Add(ctx context.Context, key string, delta int) (int, error)
}

type store struct {
	weaver.Implements[Store]
}

// NOTE: For this demonstration of simulation testing, the Store component is
// not fully implemented. It is faked in bank_test.go. A real implementation of
// the Store component would likely read from and write to a database.

// Get implements the Store interface.
func (s *store) Get(context.Context, string) (int, error) {
	return 0, fmt.Errorf("not implemented")
}

// Add implements the Store interface.
func (s *store) Add(context.Context, string, int) (int, error) {
	return 0, fmt.Errorf("not implemented")
}
