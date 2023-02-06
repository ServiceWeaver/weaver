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

package codegen

import "github.com/ServiceWeaver/weaver/metrics"

var (
	// The following metrics are automatically populated for the user.
	//
	// TODO(mwhittaker): Allow the user to disable these metrics.
	// It adds ~169ns of latency per method call.
	MethodCounts = metrics.NewCounterMap[MethodLabels](
		"serviceweaver_remote_method_count",
		"Count of Service Weaver component method invocations",
	)
	MethodErrors = metrics.NewCounterMap[MethodLabels](
		"serviceweaver_remote_method_error_count",
		"Count of Service Weaver component method invocations that result in an error",
	)
	MethodLatencies = metrics.NewHistogramMap[MethodLabels](
		"serviceweaver_remote_method_latency_micros",
		"Duration, in microseconds, of Service Weaver component method execution",
		metrics.NonNegativeBuckets,
	)
	MethodBytesRequest = metrics.NewHistogramMap[MethodLabels](
		"serviceweaver_remote_method_bytes_request",
		"Number of bytes in Service Weaver component method requests",
		metrics.NonNegativeBuckets,
	)
	MethodBytesReply = metrics.NewHistogramMap[MethodLabels](
		"serviceweaver_remote_method_bytes_reply",
		"Number of bytes in Service Weaver component method replies",
		metrics.NonNegativeBuckets,
	)
)

type MethodLabels struct {
	Caller    string // full calling component name
	Component string // full callee component name
	Method    string // callee component method's name
}

// MethodMetrics contains metrics for a single Service Weaver component method.
type MethodMetrics struct {
	Count        *metrics.Counter   // See MethodCounts.
	ErrorCount   *metrics.Counter   // See MethodErrors.
	Latency      *metrics.Histogram // See MethodLatencies.
	BytesRequest *metrics.Histogram // See MethodBytesRequest.
	BytesReply   *metrics.Histogram // See MethodBytesReply.
}

// MethodMetricsFor returns metrics for the specified method.
func MethodMetricsFor(labels MethodLabels) *MethodMetrics {
	return &MethodMetrics{
		Count:        MethodCounts.Get(labels),
		ErrorCount:   MethodErrors.Get(labels),
		Latency:      MethodLatencies.Get(labels),
		BytesRequest: MethodBytesRequest.Get(labels),
		BytesReply:   MethodBytesReply.Get(labels),
	}
}
