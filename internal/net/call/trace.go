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

	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"go.opentelemetry.io/otel/trace"
)

// writeTraceContext serializes the trace context (if any) contained in ctx
// into enc.
func writeTraceContext(ctx context.Context, enc *codegen.Encoder) {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		enc.Bool(false)
		return
	}
	enc.Bool(true)

	// Send trace information in the header.
	// TODO(spetrovic): Confirm that we don't need to bother with TraceState,
	// which seems to be used for storing vendor-specific information.
	traceID := sc.TraceID()
	spanID := sc.SpanID()
	copy(enc.Grow(len(traceID)), traceID[:])
	copy(enc.Grow(len(spanID)), spanID[:])
	enc.Byte(byte(sc.TraceFlags()))
}

// readTraceContext returns a span context with tracing information stored in dec.
func readTraceContext(dec *codegen.Decoder) *trace.SpanContext {
	hasTrace := dec.Bool()
	if !hasTrace {
		return nil
	}
	var traceID trace.TraceID
	var spanID trace.SpanID
	traceID = *(*trace.TraceID)(dec.Read(len(traceID)))
	spanID = *(*trace.SpanID)(dec.Read(len(spanID)))
	cfg := trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.TraceFlags(dec.Byte()),
		Remote:     true,
	}
	trace := trace.NewSpanContext(cfg)
	return &trace
}
