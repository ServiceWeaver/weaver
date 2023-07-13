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
	"crypto/tls"
	"fmt"
	"strings"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/exp/slog"
)

// remoteEnv implements the env used for non-single-process Service Weaver applications.
type remoteEnv struct {
	sysLogger *slog.Logger
	conn      *conn.WeaveletConn
	logmu     sync.Mutex
}

var _ env = &remoteEnv{}

func newRemoteEnv(_ context.Context, bootstrap runtime.Bootstrap, handler conn.WeaveletHandler) (*remoteEnv, error) {
	// Create pipe to communicate with the envelope.
	toWeavelet, toEnvelope, err := bootstrap.MakePipes()
	if err != nil {
		return nil, err
	}
	conn, err := conn.NewWeaveletConn(toWeavelet, toEnvelope, handler)
	if err != nil {
		return nil, fmt.Errorf("new weavelet conn: %w", err)
	}
	info := conn.EnvelopeInfo()

	env := &remoteEnv{
		conn: conn,
	}
	env.sysLogger = slog.New(&logging.LogHandler{
		Opts: logging.Options{
			App:        info.App,
			Deployment: info.DeploymentId,
			Component:  "weavelet",
			Weavelet:   info.Id,
			Attrs:      []string{"serviceweaver/system", ""},
		},
		Write: env.CreateLogSaver(),
	})
	return env, nil
}

// EnvelopeInfo implements the Env interface.
func (e *remoteEnv) EnvelopeInfo() *protos.EnvelopeInfo {
	return e.conn.EnvelopeInfo()
}

// ServeWeaveletConn implements the Env interface.
func (e *remoteEnv) ServeWeaveletConn() error {
	return e.conn.Serve()
}

// ActivateComponent implements the Env interface.
func (e *remoteEnv) ActivateComponent(_ context.Context, component string, routed bool) error {
	request := &protos.ActivateComponentRequest{
		Component: component,
		Routed:    routed,
	}
	return e.conn.ActivateComponentRPC(request)
}

// GetListenerAddress implements the Env interface.
func (e *remoteEnv) GetListenerAddress(_ context.Context, name string) (*protos.GetListenerAddressReply, error) {
	request := &protos.GetListenerAddressRequest{Name: name}
	return e.conn.GetListenerAddressRPC(request)
}

// ExportListener implements the Env interface.
func (e *remoteEnv) ExportListener(_ context.Context, listener, addr string) (*protos.ExportListenerReply, error) {
	request := &protos.ExportListenerRequest{
		Listener: listener,
		Address:  addr,
	}
	return e.conn.ExportListenerRPC(request)
}

// GetSelfCertificate implements the Env interface.
func (e *remoteEnv) GetSelfCertificate(context.Context) ([]byte, []byte, error) {
	request := &protos.GetSelfCertificateRequest{}
	reply, err := e.conn.GetSelfCertificateRPC(request)
	if err != nil {
		return nil, nil, err
	}
	return reply.Cert, reply.Key, nil
}

// VerifyClientCertificate implements the Env interface.
func (e *remoteEnv) VerifyClientCertificate(_ context.Context, certChain [][]byte) ([]string, error) {
	request := &protos.VerifyClientCertificateRequest{CertChain: certChain}
	reply, err := e.conn.VerifyClientCertificateRPC(request)
	if err != nil {
		return nil, err
	}
	return reply.Components, nil
}

// VerifyServerCertificate implements the Env interface.
func (e *remoteEnv) VerifyServerCertificate(_ context.Context, certChain [][]byte, targetComponent string) error {
	request := &protos.VerifyServerCertificateRequest{
		CertChain:       certChain,
		TargetComponent: targetComponent,
	}
	return e.conn.VerifyServerCertificateRPC(request)
}

// CreateLogSaver implements the Env interface.
func (e *remoteEnv) CreateLogSaver() func(entry *protos.LogEntry) {
	return func(entry *protos.LogEntry) {
		// Hold lock while creating entry and writing so that records are
		// printed in timestamp order, and without their contents getting
		// jumbled together.
		e.logmu.Lock()
		defer e.logmu.Unlock()
		e.conn.SendLogEntry(entry) //nolint:errcheck // TODO(mwhittaker): Propagate error.
	}
}

// SystemLogger implements the Env interface.
func (e *remoteEnv) SystemLogger() *slog.Logger {
	return e.sysLogger
}

// CreateTraceExporter implements the Env interface.
func (e *remoteEnv) CreateTraceExporter() sdktrace.SpanExporter {
	return traceio.NewWriter(e.conn.SendTraceSpans)
}

// parseEndpoints parses a list of endpoint addresses into a list of
// call.Endpoints.
func parseEndpoints(addrs []string, config *tls.Config) ([]call.Endpoint, error) {
	var endpoints []call.Endpoint
	var err error
	var ep call.Endpoint
	for _, addr := range addrs {
		const mtlsPrefix = "mtls://"
		if ep, err = call.ParseNetEndpoint(strings.TrimPrefix(addr, mtlsPrefix)); err != nil {
			return nil, err
		}
		if strings.HasPrefix(addr, mtlsPrefix) {
			if config == nil {
				return nil, fmt.Errorf("mtls protocol requires a non-nil TLS config")
			}
			ep = call.MTLS(config, ep)
		}
		endpoints = append(endpoints, ep)
	}
	return endpoints, nil
}
