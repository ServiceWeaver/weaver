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

package weaver

import (
	"context"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/exp/slog"
)

// env provides the API by which a Service Weaver application process communicates with
// its execution environment, e.g., to do thing like starting processes etc.
type env interface {
	// EnvelopeInfo returns the EnvelopeInfo sent by the envelope.
	EnvelopeInfo() *protos.EnvelopeInfo

	// ActivateComponent ensures that the provided component is running somewhere.
	ActivateComponent(ctx context.Context, component string, routed bool) error

	// GetListenerAddress returns the address a weavelet should listen on for a listener.
	GetListenerAddress(ctx context.Context, listener string, opts ListenerOptions) (*protos.GetListenerAddressReply, error)

	// ExportListener exports a listener.
	ExportListener(ctx context.Context, listener, addr string, opts ListenerOptions) (*protos.ExportListenerReply, error)

	// CreateLogSaver creates and returns a function that saves log entries
	// to the environment.
	CreateLogSaver() func(entry *protos.LogEntry)

	// SystemLogger returns the Logger for system messages.
	SystemLogger() *slog.Logger

	// CreateTraceExporter returns an exporter that should be used for
	// exporting trace spans. A nil exporter value means that no traces
	// should be exported.
	CreateTraceExporter() sdktrace.SpanExporter
}

// getEnv returns the env to use for this weavelet.
func getEnv(ctx context.Context, handler conn.WeaveletHandler) (env, error) {
	bootstrap, err := runtime.GetBootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if !bootstrap.HasPipes() {
		return newSingleprocessEnv(bootstrap)
	}
	return newRemoteEnv(ctx, bootstrap, handler)
}
