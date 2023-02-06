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

package main

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"
	weaver "github.com/ServiceWeaver/weaver"
)

// Even computes the next value in the collatz sequence for a even integers.
type Even interface {
	// Do(ctx, x) returns x/2. x must be positive and even.
	Do(context.Context, int) (int, error)
}

type even struct {
	weaver.Implements[Even]
}

func (e *even) Do(ctx context.Context, x int) (int, error) {
	trace.SpanFromContext(ctx).AddEvent(fmt.Sprintf("even.Do(%d)", x))
	if x <= 0 {
		return 0, fmt.Errorf("%d is not positive", x)
	}
	if x%2 != 0 {
		return 0, fmt.Errorf("%d is not even", x)
	}
	return x / 2, nil
}
