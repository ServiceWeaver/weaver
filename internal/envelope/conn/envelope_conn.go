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
	"context"
	"fmt"
	"io"
	sync "sync"

	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/sync/errgroup"
)

// EnvelopeHandler implements the envelope side processing of messages
// exchanged with the managed weavelet.
type EnvelopeHandler interface {
	// StartComponent starts the given component.
	StartComponent(entry *protos.ComponentToStart) error

	// GetAddress gets the address a weavelet should listen on for a listener.
	GetAddress(req *protos.GetAddressRequest) (*protos.GetAddressReply, error)

	// ExportListener exports the given listener.
	ExportListener(req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error)

	// RecvLogEntry enables the envelope to receive a log entry.
	RecvLogEntry(entry *protos.LogEntry)

	// RecvTraceSpans enables the envelope to receive a sequence of trace spans.
	RecvTraceSpans(spans []trace.ReadOnlySpan) error
}

// EnvelopeConn is the envelope side of the connection between a weavelet
// and the envelope.
type EnvelopeConn struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	conn      conn
	metrics   metrics.Importer
	weavelet  *protos.WeaveletInfo
}

// NewEnvelopeConn creates the connection to an (already started) weavelet
// and starts accepting messages from it. The connection uses (r,w) to carry
// messages. Synthesized high-level events are passed to h.
//
// The connection stops on error or when the provided context is canceled.
func NewEnvelopeConn(ctx context.Context, r io.ReadCloser, w io.WriteCloser, wlet *protos.WeaveletSetupInfo) (*EnvelopeConn, error) {
	ctx, cancel := context.WithCancel(ctx)
	e := &EnvelopeConn{
		ctx:       ctx,
		ctxCancel: cancel,
		conn:      conn{name: "envelope", reader: r, writer: w},
	}
	// Send the setup information to the weavelet, and receive the weavelet
	// information in return.
	if err := e.conn.send(&protos.EnvelopeMsg{WeaveletSetupInfo: wlet}); err != nil {
		e.conn.cleanup(err)
		return nil, err
	}
	reply := &protos.WeaveletMsg{}
	if err := e.conn.recv(reply); err != nil {
		e.conn.cleanup(err)
		return nil, err
	}
	if reply.WeaveletInfo == nil {
		err := fmt.Errorf(
			"the first message from the weavelet must contain weavelet info")
		e.conn.cleanup(err)
		return nil, err
	}
	e.weavelet = reply.WeaveletInfo
	return e, nil
}

// Serve accepts incoming messages from the weavelet. Messages that are received
// are handled as an ordered sequence. This call blocks until the connection
// terminates, returning the error that caused it to terminate. This method will
// never return a non-nil error.
func (e *EnvelopeConn) Serve(h EnvelopeHandler) error {
	msgs := make(chan *protos.WeaveletMsg, 100)
	var running errgroup.Group

	var stopErr error
	var once sync.Once
	stop := func(err error) {
		once.Do(func() {
			stopErr = err
		})
		e.ctxCancel()
		e.conn.cleanup(err)
	}

	// Spawn a goroutine that repeatedly reads messages from the pipe. A
	// received message is either an RPC response or an RPC request. conn.recv
	// handles RPC responses internally but returns all RPC requests. The
	// reading goroutine forwards those requests to the goroutine spawned below
	// for execution.
	//
	// We have to split receiving requests and processing requests across two
	// different goroutines to avoid deadlocking. Assume for contradiction that
	// we called conn.recv and handleMessage in the same goroutine:
	//
	//     for {
	//         msg := &protos.WeaveletMsg{}
	//         e.conn.recv(msg)
	//         e.handleMessage(msg)
	//     }
	//
	// If an EnvelopeHandler, invoked by handleMessage, calls an RPC on the
	// weavelet (e.g., GetHealth), then it will block forever, as the RPC
	// response will never be read by conn.recv.
	//
	// TODO(mwhittaker): I think we may be able to clean up this code if we use
	// four pipes instead of two. The four pipes will be divided into two
	// pairs. One pair will be used for RPCs from the envelope to the weavelet
	// (e.g., GetHealth, GetMetrics, GetLoad, UpdateComponents,
	// UpdateRoutingInfo). The other pair will be used for RPCs from the
	// weavelet to the envelope (e.g., StartComponent, RegisterReplica,
	// RecvLogEntry).
	running.Go(func() error {
		for {
			msg := &protos.WeaveletMsg{}
			if err := e.conn.recv(msg); err != nil {
				stop(err)
				return err
			}
			msgs <- msg
		}
	})

	// Spawn a goroutine to handle RPC requests. Note that we don't spawn one
	// goroutine for every request because we must guarantee that requests are
	// processed in order. Logs, for example, need to be received and processed
	// in order.
	running.Go(func() error {
		for {
			select {
			case msg := <-msgs:
				if err := e.handleMessage(msg, h); err != nil {
					stop(err)
					return err
				}
			case <-e.ctx.Done():
				stop(e.ctx.Err())
				return e.ctx.Err()
			}
		}
	})

	running.Wait() //nolint:errcheck // supplanted by stopErr
	return stopErr
}

// WeaveletInfo returns the information about the weavelet.
func (e *EnvelopeConn) WeaveletInfo() *protos.WeaveletInfo {
	return e.weavelet
}

// handleMessage handles all messages initiated by the weavelet. Note that
// this method doesn't handle RPC reply messages sent over by the weavelet.
func (e *EnvelopeConn) handleMessage(msg *protos.WeaveletMsg, h EnvelopeHandler) error {
	errReply := func(err error) *protos.EnvelopeMsg {
		var errStr string
		if err != nil {
			errStr = err.Error()
		}
		return &protos.EnvelopeMsg{Id: -msg.Id, Error: errStr}
	}
	switch {
	case msg.ComponentToStart != nil:
		return e.conn.send(errReply(h.StartComponent(msg.ComponentToStart)))
	case msg.GetAddressRequest != nil:
		reply, err := h.GetAddress(msg.GetAddressRequest)
		if err != nil {
			return e.conn.send(errReply(err))
		}
		return e.conn.send(&protos.EnvelopeMsg{Id: -msg.Id, GetAddressReply: reply})
	case msg.ExportListenerRequest != nil:
		reply, err := h.ExportListener(msg.ExportListenerRequest)
		if err != nil {
			// Reply with error.
			return e.conn.send(errReply(err))
		}
		// Reply with listener info.
		return e.conn.send(&protos.EnvelopeMsg{Id: -msg.Id, ExportListenerReply: reply})
	case msg.LogEntry != nil:
		h.RecvLogEntry(msg.LogEntry)
		return nil
	case msg.TraceSpans != nil:
		traces := make([]trace.ReadOnlySpan, len(msg.TraceSpans.Span))
		for i, span := range msg.TraceSpans.Span {
			traces[i] = &traceio.ReadSpan{Span: span}
		}
		return h.RecvTraceSpans(traces)
	default:
		err := fmt.Errorf("envelope_conn: unexpected message %+v", msg)
		e.conn.cleanup(err)
		return err
	}
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

// GetLoadInfoRPC requests the weavelet to return the latest load information.
func (e *EnvelopeConn) GetLoadInfoRPC() (*protos.WeaveletLoadReport, error) {
	reply, err := e.rpc(&protos.EnvelopeMsg{SendLoadInfo: true})
	if err != nil {
		return nil, err
	}
	if reply.LoadReport == nil {
		return nil, fmt.Errorf("nil load info reply received from weavelet")
	}
	return reply.LoadReport, nil
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

// UpdateComponentsRPC updates the weavelet with the latest set of components
// it should be running.
func (e *EnvelopeConn) UpdateComponentsRPC(req *protos.ComponentsToStart) error {
	response, err := e.conn.rpc(&protos.EnvelopeMsg{ComponentsToStart: req})
	if err != nil {
		err := fmt.Errorf("connection to weavelet broken: %w", err)
		e.conn.cleanup(err)
		return err
	}
	msg, ok := response.(*protos.WeaveletMsg)
	if !ok {
		return fmt.Errorf("response has wrong type %T", response)
	}
	if msg.Error != "" {
		return fmt.Errorf(msg.Error)
	}
	return nil
}

// UpdateRoutingInfoRPC updates the weavelet with a component's most recent
// routing info.
func (e *EnvelopeConn) UpdateRoutingInfoRPC(req *protos.RoutingInfo) error {
	response, err := e.conn.rpc(&protos.EnvelopeMsg{RoutingInfo: req})
	if err != nil {
		err := fmt.Errorf("connection to weavelet broken: %w", err)
		e.conn.cleanup(err)
		return err
	}
	msg, ok := response.(*protos.WeaveletMsg)
	if !ok {
		return fmt.Errorf("response has wrong type %T", response)
	}
	if msg.Error != "" {
		return fmt.Errorf(msg.Error)
	}
	return nil
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
