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

package traceio

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	// Trace attribute keys for various Service Weaver identifiers. These
	// are attached to all exported traces by the weavelet, and displayed
	// in the UI by the Service Weaver visualization tools (e.g., dashboard).
	AppTraceKey          = attribute.Key("serviceweaver.app")
	DeploymentIdTraceKey = attribute.Key("serviceweaver.deployment_id")
	WeaveletIdTraceKey   = attribute.Key("serviceweaver.weavelet_id")
)

// TestTracer returns a simple tracer suitable for tests.
func TestTracer() trace.Tracer {
	exporter, _ := stdouttrace.New(stdouttrace.WithPrettyPrint())
	return sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter)).Tracer("test")
}
