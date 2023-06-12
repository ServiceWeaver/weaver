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

// env provides the API by which a Service Weaver application process
// communicates with its execution environment, e.g., to do thing like starting
// processes etc.
type env interface {
	// EnvelopeInfo returns the EnvelopeInfo sent by the envelope.
	EnvelopeInfo() *protos.EnvelopeInfo

	// ActivateComponent ensures that the provided component is running
	// somewhere.
	ActivateComponent(ctx context.Context, component string, routed bool) error

	// GetListenerAddress returns the address a weavelet should listen on for
	// a listener.
	GetListenerAddress(ctx context.Context, listener string) (*protos.GetListenerAddressReply, error)

	// ExportListener exports a listener.
	ExportListener(ctx context.Context, listener, addr string) (*protos.ExportListenerReply, error)

	// GetSelfCertificate returns the certificate and the private key the
	// weavelet should use for network connection establishment.
	GetSelfCertificate(ctx context.Context) ([]byte, []byte, error)

	// VerifyClientCertificate verifies the certificate chain presented by
	// a network client attempting to connect to the weavelet. It returns an
	// error if the network connection should not be established with the
	// client. Otherwise, it returns the list of weavelet components that the
	// client is authorized to invoke methods on.
	//
	// NOTE: this method is only called if weavelet security was enabled, i.e.,
	// if EnvelopeInfo had non-nil SelfKey and SelfCertChain fields.
	VerifyClientCertificate(ctx context.Context, certChain [][]byte) ([]string, error)

	// VerifyServerCertificate verifies the certificate chain presented by
	// the server the weavelet is attempting to connect to. It returns an
	// error iff the server identity doesn't match the identity of the passed
	// component.
	//
	// NOTE: this method is only called if weavelet security was enabled, i.e.,
	// if EnvelopeInfo had non-nil SelfKey and SelfCert fields.
	VerifyServerCertificate(ctx context.Context, certChain [][]byte, targetComponent string) error

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
