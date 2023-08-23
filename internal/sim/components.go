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

	"github.com/ServiceWeaver/weaver"
)

//go:generate ../../cmd/weaver/weaver generate .

// This file contains components used to test the simulator in sim_test.go.

type swapper interface {
	Swap(context.Context, int, int) (int, int, error)
}

type swapperImpl struct {
	weaver.Implements[swapper]
}

func (s *swapperImpl) Swap(_ context.Context, x, y int) (int, int, error) {
	return y, x, swapperError{}
}

type swapperError struct {
	weaver.AutoMarshal
}

func (swapperError) Error() string {
	return "swapperError"
}
