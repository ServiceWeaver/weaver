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
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceSerialization(t *testing.T) {
	// Create a random trace context.
	rndBytes := func() []byte {
		b := uuid.New()
		return b[:]
	}
	span := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID(uuid.New()),
		SpanID:     *(*trace.SpanID)(rndBytes()[:8]),
		TraceFlags: trace.TraceFlags(rndBytes()[0]),
	})

	// Serialize the trace context.
	var b [25]byte
	writeTraceContext(
		trace.ContextWithSpanContext(context.Background(), span), b[:])

	// Deserialize the trace context.
	actual := readTraceContext(b[:])
	expect := span.WithRemote(true)
	if !expect.Equal(actual) {
		want, _ := json.Marshal(expect)
		got, _ := json.Marshal(actual)
		t.Errorf("span context diff, want %q, got %q", want, got)
	}
}
