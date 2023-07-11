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

import (
	"time"

	"github.com/ServiceWeaver/weaver/metrics"
)

// Names of automatically populated metrics.
const (
	MethodCountsName       = "serviceweaver_method_count"
	MethodErrorsName       = "serviceweaver_method_error_count"
	MethodLatenciesName    = "serviceweaver_method_latency_micros"
	MethodBytesRequestName = "serviceweaver_method_bytes_request"
	MethodBytesReplyName   = "serviceweaver_method_bytes_reply"
)

var (
	// The following metrics are automatically populated for the user.
	methodCounts = metrics.NewCounterMap[MethodLabels](
		MethodCountsName,
		"Count of Service Weaver component method invocations",
	)
	methodErrors = metrics.NewCounterMap[MethodLabels](
		MethodErrorsName,
		"Count of Service Weaver component method invocations that result in an error",
	)
	methodLatencies = metrics.NewHistogramMap[MethodLabels](
		MethodLatenciesName,
		"Duration, in microseconds, of Service Weaver component method execution",
		metrics.NonNegativeBuckets,
	)
	methodBytesRequest = metrics.NewHistogramMap[MethodLabels](
		MethodBytesRequestName,
		"Number of bytes in Service Weaver component method requests",
		metrics.NonNegativeBuckets,
	)
	methodBytesReply = metrics.NewHistogramMap[MethodLabels](
		MethodBytesReplyName,
		"Number of bytes in Service Weaver component method replies",
		metrics.NonNegativeBuckets,
	)
)

type MethodLabels struct {
	Caller    string // full calling component name
	Component string // full callee component name
	Method    string // callee component method's name
	Remote    bool   // Is this a remote call?
}

// MethodMetrics contains metrics for a single Service Weaver component method.
type MethodMetrics struct {
	remote       bool
	Count        *metrics.Counter   // See MethodCounts.
	ErrorCount   *metrics.Counter   // See MethodErrors.
	Latency      *metrics.Histogram // See MethodLatencies.
	BytesRequest *metrics.Histogram // See MethodBytesRequest.
	BytesReply   *metrics.Histogram // See MethodBytesReply.
}

// MethodMetricsFor returns metrics for the specified method.
func MethodMetricsFor(labels MethodLabels) *MethodMetrics {
	return &MethodMetrics{
		remote:       labels.Remote,
		Count:        methodCounts.Get(labels),
		ErrorCount:   methodErrors.Get(labels),
		Latency:      methodLatencies.Get(labels),
		BytesRequest: methodBytesRequest.Get(labels),
		BytesReply:   methodBytesReply.Get(labels),
	}
}

// MethodCallHandle holds information needed to finalize metric
// updates for a method call.
type MethodCallHandle struct {
	start time.Time
}

// Begin starts metric update recording for a call to method m.
func (m *MethodMetrics) Begin() MethodCallHandle {
	return MethodCallHandle{time.Now()}
}

// End ends metric update recording for a call to method m.
func (m *MethodMetrics) End(h MethodCallHandle, failed bool, requestBytes, replyBytes int) {
	latency := time.Since(h.start).Microseconds()
	m.Count.Inc()
	if failed {
		m.ErrorCount.Inc()
	}
	m.Latency.Put(float64(latency))
	if m.remote {
		m.BytesRequest.Put(float64(requestBytes))
		m.BytesReply.Put(float64(replyBytes))
	}
}
