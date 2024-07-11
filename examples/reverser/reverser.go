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

package main

import (
	"context"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/metrics"
)

// stringLength is a histogram that tracks the length of strings passed to the
// Reverse method.
var stringLength = metrics.NewHistogram(
	"reverse_string_length",
	"The length of strings passed to the Reverse method",
	[]float64{1, 10, 100, 1000, 10000, 100000, 1000000},
)

// Reverser component.
type Reverser interface {
	Reverse(context.Context, string) (string, error)
}

// Implementation of the Reverser component.
type reverser struct {
	weaver.Implements[Reverser]
}

func (r reverser) Reverse(ctx context.Context, s string) (string, error) {
	stringLength.Put(float64(len(s))) // Update the stringLength metric.

	runes := []rune(s)
	n := len(runes)
	for i := 0; i < n/2; i++ {
		runes[i], runes[n-i-1] = runes[n-i-1], runes[i]
	}
	return string(runes), nil
}
