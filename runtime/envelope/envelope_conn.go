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

package envelope

import (
	"fmt"
	"io"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/sync/errgroup"
)

// EnvelopeConn is the envelope side of the connection between a weavelet
// and the envelope.
type EnvelopeConn struct {
	handler  EnvelopeHandler
	conn     conn.Conn[*protos.WeaveletMsg]
	weavelet *protos.WeaveletInfo
}

// NewEnvelopeConn creates the envelope side of the connection between a
// weavelet and an envelope. The connection uses (r,w) to carry messages.
// Synthesized high-level events are passed to h.
//
// NewEnvelopeConn sends the provided protos.Weavelet to the weavelet.
func NewEnvelopeConn(r io.ReadCloser, w io.WriteCloser, h EnvelopeHandler, wlet *protos.WeaveletSetupInfo) (*EnvelopeConn, error) {
	e := &EnvelopeConn{
		handler: h,
		conn:    conn.NewConn[*protos.WeaveletMsg]("envelope", r, w),
	}
	// Send the setup information to the weavelet, and receive the weavelet
	// information in return.
	if err := e.conn.Send(&protos.EnvelopeMsg{WeaveletSetupInfo: wlet}); err != nil {
		return nil, err
	}
	reply := &protos.WeaveletMsg{}
	if err := e.conn.Recv(reply); err != nil {
		e.conn.Cleanup(err)
		return nil, err
	}
	if reply.WeaveletInfo == nil {
		err := fmt.Errorf(
			"the first message from the weavelet must contain weavelet info")
		e.conn.Cleanup(err)
		return nil, err
	}
	e.weavelet = reply.WeaveletInfo
	return e, nil
}

// Serve accepts incoming messages from the weavelet. Messages that are received
// are handled as an ordered sequence.
func (e *EnvelopeConn) Serve() error {
	var group errgroup.Group
	msgs := make(chan *protos.WeaveletMsg, 100)

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
	group.Go(func() error {
		for {
			msg := &protos.WeaveletMsg{}
			if err := e.conn.Recv(msg); err != nil {
				close(msgs)
				return err
			}
			msgs <- msg
		}
	})

	// Spawn a goroutine to handle RPC requests. Note that we don't spawn one
	// goroutine for every request because we must guarantee that requests are
	// processed in order. Logs, for example, need to be received and processed
	// in order.
	group.Go(func() error {
		for {
			msg, ok := <-msgs
			if !ok {
				return nil
			}
			if err := e.handleMessage(msg); err != nil {
				e.conn.Cleanup(err)
				return err
			}
		}
	})

	return group.Wait()
}

// WeaveletInfo returns the information about the weavelet.
func (e *EnvelopeConn) WeaveletInfo() *protos.WeaveletInfo {
	return e.weavelet
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
		return e.conn.Send(errReply(e.handler.StartComponent(msg.ComponentToStart)))
	case msg.GetAddressRequest != nil:
		reply, err := e.handler.GetAddress(msg.GetAddressRequest)
		if err != nil {
			return e.conn.Send(errReply(err))
		}
		return e.conn.Send(&protos.EnvelopeMsg{Id: -msg.Id, GetAddressReply: reply})
	case msg.ExportListenerRequest != nil:
		reply, err := e.handler.ExportListener(msg.ExportListenerRequest)
		if err != nil {
			// Reply with error.
			return e.conn.Send(errReply(err))
		}
		// Reply with listener info.
		return e.conn.Send(&protos.EnvelopeMsg{Id: -msg.Id, ExportListenerReply: reply})
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
		err := fmt.Errorf("envelope_conn: unexpected message %+v", msg)
		e.conn.Cleanup(err)
		return err
	}
}
