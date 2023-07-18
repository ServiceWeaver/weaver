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

package weaver

import (
	"fmt"
	"os"

	"github.com/ServiceWeaver/weaver/internal/traceio"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

// tracer returns a tracer for the provided app, deploymentId, and weaveletId
// that uses the provided exporter. The tracer is also set as the otel default.
func tracer(exporter sdktrace.SpanExporter, app, deploymentId, weaveletId string) trace.Tracer {
	const instrumentationLibrary = "github.com/ServiceWeaver/weaver/serviceweaver"
	const instrumentationVersion = "0.0.1"
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(fmt.Sprintf("serviceweaver/%s", weaveletId)),
			semconv.ProcessPIDKey.Int(os.Getpid()),
			traceio.AppTraceKey.String(app),
			traceio.DeploymentIdTraceKey.String(deploymentId),
			traceio.WeaveletIdTraceKey.String(weaveletId),
		)),
		// TODO(spetrovic): Allow the user to create new TracerProviders where
		// they can control trace sampling and other options.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())))
	tracer := tracerProvider.Tracer(instrumentationLibrary, trace.WithInstrumentationVersion(instrumentationVersion))

	// Set global tracing defaults.
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tracer
}
