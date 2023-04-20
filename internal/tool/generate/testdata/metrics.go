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

// EXPECTED
// codegen.MethodMetricsFor(
// codegen.MethodLabels{
// methodMetrics *codegen.MethodMetrics
// start := time.Now()
// s.methodMetrics.Count.Add(1)
// s.methodMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
// s.methodMetrics.BytesRequest.Put(float64(len(enc.Data())))
// s.methodMetrics.BytesReply.Put(float64(len(results)))

package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type foo interface {
	Method(context.Context, string) (string, error)
}

type impl struct{ weaver.Implements[foo] }

func (i *impl) Method(context.Context, string) (string, error) {
	return "", nil
}
