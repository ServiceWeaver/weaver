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
	"fmt"
	"net"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/logtype"
	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// remoteEnv implements the env used for non-single-process Service Weaver applications.
type remoteEnv struct {
	info      *protos.WeaveletSetupInfo
	sysLogger Logger
	conn      *conn.WeaveletConn
	logmu     sync.Mutex
}

var _ env = &remoteEnv{}

func newRemoteEnv(ctx context.Context, bootstrap runtime.Bootstrap, handler conn.WeaveletHandler) (*remoteEnv, error) {
	// Create pipe to communicate with the envelope.
	toWeavelet, toEnvelope, err := bootstrap.MakePipes()
	if err != nil {
		return nil, err
	}
	conn, err := conn.NewWeaveletConn(toWeavelet, toEnvelope, handler)
	if err != nil {
		return nil, fmt.Errorf("new weavelet conn: %w", err)
	}
	info := conn.WeaveletSetupInfo()

	env := &remoteEnv{
		info: info,
		conn: conn,
	}

	logSaver := env.CreateLogSaver()
	env.sysLogger = newAttrLogger(info.App, info.DeploymentId, "weavelet", info.Id, logSaver)
	env.sysLogger = env.sysLogger.With("serviceweaver/system", "")
	return env, nil
}

// WeaveletSetupInfo implements the Env interface.
func (e *remoteEnv) WeaveletSetupInfo() *protos.WeaveletSetupInfo {
	return e.info
}

// WeaveletListener implements the Env interface.
func (e *remoteEnv) WeaveletListener() net.Listener {
	return e.conn.Listener()
}

// ServeWeaveletConn implements the Env interface.
func (e *remoteEnv) ServeWeaveletConn() error {
	return e.conn.Serve()
}

// RegisterComponentToStart implements the Env interface.
func (e *remoteEnv) RegisterComponentToStart(_ context.Context, component string, routed bool) error {
	request := &protos.ComponentToStart{
		Component: component,
		Routed:    routed,
	}
	return e.conn.StartComponentRPC(request)
}

// GetAddress implements the Env interface.
func (e *remoteEnv) GetAddress(_ context.Context, name string, opts ListenerOptions) (*protos.GetAddressReply, error) {
	request := &protos.GetAddressRequest{
		Name:         name,
		LocalAddress: opts.LocalAddress,
	}
	return e.conn.GetAddressRPC(request)
}

// ExportListener implements the Env interface.
func (e *remoteEnv) ExportListener(_ context.Context, lis *protos.Listener, opts ListenerOptions) (*protos.ExportListenerReply, error) {
	request := &protos.ExportListenerRequest{
		Listener:     lis,
		LocalAddress: opts.LocalAddress,
	}
	return e.conn.ExportListenerRPC(request)
}

// CreateLogSaver implements the Env interface.
func (e *remoteEnv) CreateLogSaver() func(entry *protos.LogEntry) {
	return func(entry *protos.LogEntry) {
		// Hold lock while creating entry and writing so that records are
		// printed in timestamp order, and without their contents getting
		// jumbled together.
		e.logmu.Lock()
		defer e.logmu.Unlock()
		e.conn.SendLogEntry(entry)
	}
}

// SystemLogger implements the Env interface.
func (e *remoteEnv) SystemLogger() logtype.Logger {
	return e.sysLogger
}

// CreateTraceExporter implements the Env interface.
func (e *remoteEnv) CreateTraceExporter() sdktrace.SpanExporter {
	return traceio.NewWriter(e.conn.SendTraceSpans)
}

// parseEndpoints parses a list of endpoint addresses into a list of
// call.Endpoints.
func parseEndpoints(addrs []string) ([]call.Endpoint, error) {
	var endpoints []call.Endpoint
	for _, addr := range addrs {
		endpoint, err := parseEndpoint(addr)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints, nil
}

// parseEndpoint parses an endpoint address into a call.Endpoint.
func parseEndpoint(endpoint string) (call.Endpoint, error) {
	net, addr, err := call.NetworkAddress(endpoint).Split()
	if err != nil {
		return nil, fmt.Errorf("bad endpoint %q: %w", endpoint, err)
	}
	return call.NetEndpoint{Net: net, Addr: addr}, nil
}
