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

package conn

import (
	"fmt"
	"io"

	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"go.opentelemetry.io/otel/sdk/trace"
)

// EnvelopeHandler implements the envelope side processing of messages
// exchanged with the managed weavelet.
type EnvelopeHandler interface {
	// StartComponent starts the given component.
	StartComponent(entry *protos.ComponentToStart) error

	// StartColocationGroup starts the given colocation group.
	StartColocationGroup(entry *protos.ColocationGroup) error

	// RegisterReplica registers the given weavelet replica.
	RegisterReplica(entry *protos.ReplicaToRegister) error

	// ReportLoad reports the given weavelet load information.
	ReportLoad(entry *protos.WeaveletLoadReport) error

	// GetAddress gets the address a weavelet should listen on for a listener.
	GetAddress(req *protos.GetAddressRequest) (*protos.GetAddressReply, error)

	// ExportListener exports the given listener.
	ExportListener(req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error)

	// GetRoutingInfo returns the latest routing information for the weavelet.
	//
	// This is a blocking method that can be processed out-of-order w.r.t.
	// the other methods.
	GetRoutingInfo(request *protos.GetRoutingInfo) (*protos.RoutingInfo, error)

	// GetComponentsToStart is a blocking call that returns the latest set of
	// components that should be started by the weavelet.
	//
	// This is a blocking method that can be processed out-of-order w.r.t.
	// the other methods.
	GetComponentsToStart(request *protos.GetComponentsToStart) (*protos.ComponentsToStart, error)

	// RecvLogEntry enables the envelope to receive a log entry.
	RecvLogEntry(entry *protos.LogEntry)

	// RecvTraceSpans enables the envelope to receive a sequence of trace spans.
	RecvTraceSpans(spans []trace.ReadOnlySpan) error
}

// EnvelopeConn is the envelope side of the connection between a weavelet
// and the envelope.
type EnvelopeConn struct {
	handler EnvelopeHandler
	conn    conn
	metrics metrics.Importer
}

// NewEnvelopeConn creates the envelope side of the connection between a
// weavelet and an envelope. The connection uses (r,w) to carry messages.
// Synthesized high-level events are passed to h.
//
// NewEnvelopeConn sends the provided protos.Weavelet to the weavelet.
func NewEnvelopeConn(r io.ReadCloser, w io.WriteCloser, h EnvelopeHandler, weavelet *protos.WeaveletInfo) (*EnvelopeConn, error) {
	e := &EnvelopeConn{
		handler: h,
		conn:    conn{name: "envelope", reader: r, writer: w},
	}

	req := &protos.EnvelopeMsg{WeaveletInfo: weavelet}
	err := e.send(req)
	return e, err
}

// Run interacts with the peer. Messages that are received are
// handled as an ordered sequence.
func (e *EnvelopeConn) Run() error {
	msg := &protos.WeaveletMsg{}
	for {
		if err := e.conn.recv(msg); err != nil {
			return err
		}
		if err := e.handleMessage(msg); err != nil {
			return err
		}
	}
}

// handleMessage handles all messages initiated by the weavelet. Note that
// this method doesn't handle RPC reply messages sent over by the weavelet.
func (e *EnvelopeConn) handleMessage(msg *protos.WeaveletMsg) error {
	errReply := func(err error) *protos.EnvelopeMsg {
		var errStr string
		if err != nil {
			errStr = err.Error()
		}
		return &protos.EnvelopeMsg{Id: -msg.Id, Error: errStr}
	}
	switch {
	case msg.ComponentToStart != nil:
		return e.send(errReply(e.handler.StartComponent(msg.ComponentToStart)))
	case msg.ColocationGroupToStart != nil:
		return e.send(errReply(e.handler.StartColocationGroup(msg.ColocationGroupToStart)))
	case msg.ReplicaToRegister != nil:
		return e.send(errReply(e.handler.RegisterReplica(msg.ReplicaToRegister)))
	case msg.LoadReport != nil:
		return e.send(errReply(e.handler.ReportLoad(msg.LoadReport)))
	case msg.GetAddressRequest != nil:
		reply, err := e.handler.GetAddress(msg.GetAddressRequest)
		if err != nil {
			return e.send(errReply(err))
		}
		return e.send(&protos.EnvelopeMsg{Id: -msg.Id, GetAddressReply: reply})
	case msg.ExportListenerRequest != nil:
		reply, err := e.handler.ExportListener(msg.ExportListenerRequest)
		if err != nil {
			// Reply with error.
			return e.send(errReply(err))
		}
		// Reply with listener info.
		return e.send(&protos.EnvelopeMsg{Id: -msg.Id, ExportListenerReply: reply})
	case msg.GetRoutingInfo != nil:
		// This is a blocking call and therefore we cannot process it inline:
		// process it in a separate goroutine. Note that this will cause routing
		// info requests to be processed out-of-order w.r.t. other messages.
		id := msg.Id
		request := protomsg.Clone(msg.GetRoutingInfo)
		go func() {
			info, err := e.handler.GetRoutingInfo(request)
			if err != nil {
				// Reply with error.
				e.send(&protos.EnvelopeMsg{Id: -id, Error: err.Error()})
				return
			}
			// Reply with routing info.
			e.send(&protos.EnvelopeMsg{Id: -id, RoutingInfo: info})
		}()
		return nil
	case msg.GetComponentsToStart != nil:
		// This is a blocking call and therefore we cannot process it inline:
		// process it in a separate goroutine. Note that this will cause routing
		// info requests to be processed out-of-order w.r.t. other messages.
		id := msg.Id
		request := protomsg.Clone(msg.GetComponentsToStart)
		go func() {
			components, err := e.handler.GetComponentsToStart(request)
			if err != nil {
				// Reply with error.
				e.send(&protos.EnvelopeMsg{Id: -id, Error: err.Error()})
				return
			}
			// Reply with components info.
			e.send(&protos.EnvelopeMsg{Id: -id, ComponentsToStart: components})
		}()
		return nil
	case msg.LogEntry != nil:
		e.handler.RecvLogEntry(msg.LogEntry)
		return nil
	case msg.TraceSpans != nil:
		traces := make([]trace.ReadOnlySpan, len(msg.TraceSpans.Span))
		for i, span := range msg.TraceSpans.Span {
			traces[i] = &traceio.ReadSpan{Span: span}
		}
		return e.handler.RecvTraceSpans(traces)
	default:
		err := fmt.Errorf("unexpected message %v", msg)
		e.conn.cleanup(err)
		return err
	}
}

func (e *EnvelopeConn) send(msg *protos.EnvelopeMsg) error {
	if err := e.conn.send(msg); err != nil {
		// Connection is broken: tear it down.
		e.conn.cleanup(err)
		return err
	}
	return nil
}

// GetMetricsRPC requests the weavelet to return its up-to-date metrics.
func (e *EnvelopeConn) GetMetricsRPC() ([]*metrics.MetricSnapshot, error) {
	reply, err := e.rpc(&protos.EnvelopeMsg{SendMetrics: true})
	if err != nil {
		return nil, err
	}
	if reply.Metrics == nil {
		return nil, fmt.Errorf("nil metrics reply received from weavelet")
	}
	return e.metrics.Import(reply.Metrics)
}

// HealthStatusRPC requests the weavelet to return its health status.
func (e *EnvelopeConn) HealthStatusRPC() (protos.HealthStatus, error) {
	reply, err := e.rpc(&protos.EnvelopeMsg{SendHealthStatus: true})
	if err != nil {
		return protos.HealthStatus_UNHEALTHY, err
	}
	if reply.HealthReport == nil {
		return protos.HealthStatus_UNHEALTHY, fmt.Errorf("nil health status reply received from weavelet")
	}
	return reply.HealthReport.Status, nil
}

// DoProfilingRPC requests the weavelet to profile itself and return its
// profile data.
func (e *EnvelopeConn) DoProfilingRPC(req *protos.RunProfiling) (*protos.Profile, error) {
	reply, err := e.rpc(&protos.EnvelopeMsg{RunProfiling: req})
	if err != nil {
		return nil, err
	}
	if reply.Profile == nil {
		return nil, fmt.Errorf("nil profile reply received from weavelet")
	}
	return reply.Profile, nil
}

func (e *EnvelopeConn) rpc(request *protos.EnvelopeMsg) (*protos.WeaveletMsg, error) {
	response, err := e.conn.rpc(request)
	if err != nil {
		err := fmt.Errorf("connection to weavelet broken: %w", err)
		e.conn.cleanup(err)
		return nil, err
	}
	msg, ok := response.(*protos.WeaveletMsg)
	if !ok {
		return nil, fmt.Errorf("response has wrong type %T", response)
	}
	if msg.Error != "" {
		return nil, fmt.Errorf(msg.Error)
	}
	return msg, nil
}
