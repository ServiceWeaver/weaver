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

package call

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

const traceHeaderLen = 25

// writeTraceContext serializes the trace context (if any) contained in ctx
// into b.
// REQUIRES: len(b) >= traceHeaderLen
func writeTraceContext(ctx context.Context, b []byte) {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return
	}

	// Send trace information in the header.
	// TODO(spetrovic): Confirm that we don't need to bother with TraceState,
	// which seems to be used for storing vendor-specific information.
	traceID := sc.TraceID()
	spanID := sc.SpanID()
	copy(b, traceID[:])
	copy(b[16:], spanID[:])
	b[24] = byte(sc.TraceFlags())
}

// readTraceContext returns a span context with tracing information stored in b.
// REQUIRES: len(b) >= traceHeaderLen
func readTraceContext(b []byte) trace.SpanContext {
	cfg := trace.SpanContextConfig{
		TraceID:    *(*trace.TraceID)(b[:16]),
		SpanID:     *(*trace.SpanID)(b[16:24]),
		TraceFlags: trace.TraceFlags(b[24]),
		Remote:     true,
	}
	return trace.NewSpanContext(cfg)
}
