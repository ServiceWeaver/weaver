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
	"sync"

	"github.com/ServiceWeaver/weaver/internal/queue"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/version"
	"golang.org/x/sync/errgroup"
)

// See envelope.EnvelopeHandler
type EnvelopeHandler interface {
	ActivateComponent(context.Context, *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error)
	GetListenerAddress(context.Context, *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error)
	ExportListener(context.Context, *protos.ExportListenerRequest) (*protos.ExportListenerReply, error)
	GetSelfCertificate(context.Context, *protos.GetSelfCertificateRequest) (*protos.GetSelfCertificateReply, error)
	VerifyClientCertificate(context.Context, *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error)
	VerifyServerCertificate(context.Context, *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error)
	HandleLogEntry(context.Context, *protos.LogEntry) error
	HandleTraceSpans(context.Context, *protos.TraceSpans) error
}

// EnvelopeConn is the envelope side of the connection between a weavelet and
// an envelope. For more information, refer to runtime/protos/runtime.proto and
// https://serviceweaver.dev/blog/deployers.html.
type EnvelopeConn struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	conn      conn
	metrics   metrics.Importer
	weavelet  *protos.WeaveletInfo
	running   errgroup.Group
	msgs      queue.Queue[*protos.WeaveletMsg]

	once sync.Once
	err  error
}

// NewEnvelopeConn returns a connection to an already started weavelet. The
// connection sends messages to and receives messages from the weavelet using r
// and w. The provided EnvelopeInfo is sent to the weavelet as part of the
// handshake.
//
// You can issue RPCs *to* the weavelet using the returned EnvelopeConn. To
// start receiving messages *from* the weavelet, call [Serve].
//
// The connection stops on error or when the provided context is canceled.
func NewEnvelopeConn(ctx context.Context, r io.ReadCloser, w io.WriteCloser, info *protos.EnvelopeInfo) (*EnvelopeConn, error) {
	ctx, cancel := context.WithCancel(ctx)
	e := &EnvelopeConn{
		ctx:       ctx,
		ctxCancel: cancel,
		conn:      conn{name: "envelope", reader: r, writer: w},
	}

	// Perform the handshake. Send EnvelopeInfo and receive WeaveletInfo.
	if err := e.conn.send(&protos.EnvelopeMsg{EnvelopeInfo: info}); err != nil {
		e.conn.cleanup(err)
		return nil, err
	}
	reply := &protos.WeaveletMsg{}
	if err := e.conn.recv(reply); err != nil {
		e.conn.cleanup(err)
		return nil, err
	}
	if err := verifyWeaveletInfo(reply.WeaveletInfo); err != nil {
		e.conn.cleanup(err)
		return nil, err
	}
	e.weavelet = reply.WeaveletInfo

	// Spawn a goroutine that repeatedly reads messages from the pipe. A
	// received message is either an RPC response or an RPC request. conn.recv
	// handles RPC responses internally but returns all RPC requests. We store
	// the returned RPC requests in a queue to be handled after Serve() is
	// called.
	//
	// There are two reasons to split the tasks of receiving requests and
	// processing requests across two different goroutines.
	//
	// The first reason is to allow envelope-issued RPCs to run and complete
	// before envelope's Serve() method has been called:
	//
	//     e, err := NewEnvelopeConn(ctx, r, w, wlet)
	//     ms, err := e.GetMetricsRPC() // should complete
	//     e.Serve()
	//
	// The second reason is to avoid deadlocking. Assume for contradiction that
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
	e.running.Go(func() error {
		for {
			msg := &protos.WeaveletMsg{}
			if err := e.conn.recv(msg); err != nil {
				e.stop(err)
				return err
			}
			e.msgs.Push(msg)
		}
	})

	// Start a goroutine that watches for context cancelation.
	// NOTE: This goroutine is redundant but useful if the caller never
	// calls e.Serve().
	e.running.Go(func() error {
		<-e.ctx.Done()
		e.stop(e.ctx.Err())
		return e.ctx.Err()
	})

	return e, nil
}

// REQUIRES: err != nil
func (e *EnvelopeConn) stop(err error) {
	e.once.Do(func() {
		e.err = err
	})

	e.ctxCancel()
	e.conn.cleanup(err)
}

// Serve accepts incoming messages from the weavelet. RPC requests are handled
// serially in the order they are received. Serve blocks until the connection
// terminates, returning the error that caused it to terminate. You can cancel
// the connection by cancelling the context passed to [NewEnvelopeConn]. This
// method never returns a non-nil error.
func (e *EnvelopeConn) Serve(h EnvelopeHandler) error {
	// Spawn a goroutine to handle envelope-issued RPC requests. Note that we
	// don't spawn one goroutine for every request because we must guarantee
	// that requests are processed in order. Logs, for example, need to be
	// received and processed in order.
	//
	// NOTE: it is possible for stop() to have already been called at this
	// point. This is fine as this goroutine will fail immediately after
	// starting.
	e.running.Go(func() error {
		for {
			// Read the next queue message.
			msg, err := e.msgs.Pop(e.ctx)
			if err != nil { // e.ctx canceled
				e.stop(err)
				return err
			}
			if err := e.handleMessage(msg, h); err != nil {
				e.stop(err)
				return err
			}
		}
	})

	e.running.Wait() //nolint:errcheck // supplanted by e.err
	return e.err
}

// WeaveletInfo returns information about the weavelet.
func (e *EnvelopeConn) WeaveletInfo() *protos.WeaveletInfo {
	return e.weavelet
}

// handleMessage handles all messages initiated by the weavelet. Note that this
// method doesn't handle RPC replies from weavelet.
func (e *EnvelopeConn) handleMessage(msg *protos.WeaveletMsg, h EnvelopeHandler) error {
	errstring := func(err error) string {
		if err == nil {
			return ""
		}
		return err.Error()
	}

	switch {
	case msg.ActivateComponentRequest != nil:
		reply, err := h.ActivateComponent(e.ctx, msg.ActivateComponentRequest)
		return e.conn.send(&protos.EnvelopeMsg{
			Id:                     -msg.Id,
			Error:                  errstring(err),
			ActivateComponentReply: reply,
		})
	case msg.GetListenerAddressRequest != nil:
		reply, err := h.GetListenerAddress(e.ctx, msg.GetListenerAddressRequest)
		return e.conn.send(&protos.EnvelopeMsg{
			Id:                      -msg.Id,
			Error:                   errstring(err),
			GetListenerAddressReply: reply,
		})
	case msg.ExportListenerRequest != nil:
		reply, err := h.ExportListener(e.ctx, msg.ExportListenerRequest)
		return e.conn.send(&protos.EnvelopeMsg{
			Id:                  -msg.Id,
			Error:               errstring(err),
			ExportListenerReply: reply,
		})
	case msg.GetSelfCertificateRequest != nil:
		reply, err := h.GetSelfCertificate(e.ctx, msg.GetSelfCertificateRequest)
		return e.conn.send(&protos.EnvelopeMsg{
			Id:                      -msg.Id,
			Error:                   errstring(err),
			GetSelfCertificateReply: reply,
		})
	case msg.VerifyClientCertificateRequest != nil:
		reply, err := h.VerifyClientCertificate(e.ctx, msg.VerifyClientCertificateRequest)
		return e.conn.send(&protos.EnvelopeMsg{
			Id:                           -msg.Id,
			Error:                        errstring(err),
			VerifyClientCertificateReply: reply,
		})
	case msg.VerifyServerCertificateRequest != nil:
		reply, err := h.VerifyServerCertificate(e.ctx, msg.VerifyServerCertificateRequest)
		return e.conn.send(&protos.EnvelopeMsg{
			Id:                           -msg.Id,
			Error:                        errstring(err),
			VerifyServerCertificateReply: reply,
		})
	case msg.LogEntry != nil:
		return h.HandleLogEntry(e.ctx, msg.LogEntry)
	case msg.TraceSpans != nil:
		return h.HandleTraceSpans(e.ctx, msg.TraceSpans)
	default:
		err := fmt.Errorf("envelope_conn: unexpected message %+v", msg)
		e.conn.cleanup(err)
		return err
	}
}

// GetMetricsRPC gets a weavelet's metrics. There can only be one outstanding
// GetMetricsRPC at a time.
func (e *EnvelopeConn) GetMetricsRPC() ([]*metrics.MetricSnapshot, error) {
	req := &protos.EnvelopeMsg{GetMetricsRequest: &protos.GetMetricsRequest{}}
	reply, err := e.rpc(req)
	if err != nil {
		return nil, err
	}
	if reply.GetMetricsReply == nil {
		return nil, fmt.Errorf("nil GetMetricsReply received from weavelet")
	}
	return e.metrics.Import(reply.GetMetricsReply.Update)
}

// GetHealthRPC gets a weavelet's health.
func (e *EnvelopeConn) GetHealthRPC() (protos.HealthStatus, error) {
	req := &protos.EnvelopeMsg{GetHealthRequest: &protos.GetHealthRequest{}}
	reply, err := e.rpc(req)
	if err != nil {
		return protos.HealthStatus_UNHEALTHY, err
	}
	if reply.GetHealthReply == nil {
		return protos.HealthStatus_UNHEALTHY, fmt.Errorf("nil HealthStatusReply received from weavelet")
	}
	return reply.GetHealthReply.Status, nil
}

// GetLoadRPC gets a load report from the weavelet.
func (e *EnvelopeConn) GetLoadRPC() (*protos.LoadReport, error) {
	req := &protos.EnvelopeMsg{GetLoadRequest: &protos.GetLoadRequest{}}
	reply, err := e.rpc(req)
	if err != nil {
		return nil, err
	}
	if reply.GetLoadReply == nil {
		return nil, fmt.Errorf("nil GetLoadReply received from weavelet")
	}
	return reply.GetLoadReply.Load, nil
}

// GetProfileRPC gets a profile from the weavelet. There can only be one
// outstanding GetProfileRPC at a time.
func (e *EnvelopeConn) GetProfileRPC(req *protos.GetProfileRequest) ([]byte, error) {
	reply, err := e.rpc(&protos.EnvelopeMsg{GetProfileRequest: req})
	if err != nil {
		return nil, err
	}
	if reply.GetProfileReply == nil {
		return nil, fmt.Errorf("nil GetProfileReply received from weavelet")
	}
	return reply.GetProfileReply.Data, nil
}

// UpdateComponentsRPC updates the weavelet with the latest set of components
// it should be running.
func (e *EnvelopeConn) UpdateComponentsRPC(components []string) error {
	req := &protos.EnvelopeMsg{
		UpdateComponentsRequest: &protos.UpdateComponentsRequest{
			Components: components,
		},
	}
	reply, err := e.rpc(req)
	if err != nil {
		return err
	}
	if reply.UpdateComponentsReply == nil {
		return fmt.Errorf("nil UpdateComponentsReply received from weavelet")
	}
	return nil
}

// UpdateRoutingInfoRPC updates the weavelet with a component's most recent
// routing info.
func (e *EnvelopeConn) UpdateRoutingInfoRPC(routing *protos.RoutingInfo) error {
	req := &protos.EnvelopeMsg{
		UpdateRoutingInfoRequest: &protos.UpdateRoutingInfoRequest{
			RoutingInfo: routing,
		},
	}
	reply, err := e.rpc(req)
	if err != nil {
		return err
	}
	if reply.UpdateRoutingInfoReply == nil {
		return fmt.Errorf("nil UpdateRoutingInfoReply received from weavelet")
	}
	return nil
}

func (e *EnvelopeConn) rpc(request *protos.EnvelopeMsg) (*protos.WeaveletMsg, error) {
	response, err := e.conn.doBlockingRPC(request)
	if err != nil {
		err := fmt.Errorf("connection to weavelet broken: %w", err)
		e.conn.cleanup(err)
		return nil, err
	}
	msg, ok := response.(*protos.WeaveletMsg)
	if !ok {
		return nil, fmt.Errorf("weavelet response has wrong type %T", response)
	}
	if msg.Error != "" {
		return nil, fmt.Errorf(msg.Error)
	}
	return msg, nil
}

// verifyWeaveletInfo verifies the information sent by the weavelet.
func verifyWeaveletInfo(wlet *protos.WeaveletInfo) error {
	if wlet == nil {
		return fmt.Errorf(
			"the first message from the weavelet must contain weavelet info")
	}
	if wlet.DialAddr == "" {
		return fmt.Errorf("empty dial address for the weavelet")
	}
	if err := checkVersion(wlet.Version); err != nil {
		return err
	}
	return nil
}

// checkVersion checks that the deployer API version the deployer was built
// with is compatible with the deployer API version the app was built with,
// erroring out if they are not compatible.
func checkVersion(v *protos.SemVer) error {
	if v == nil {
		return fmt.Errorf("version mismatch: nil app version")
	}
	got := version.SemVer{Major: int(v.Major), Minor: int(v.Minor), Patch: int(v.Patch)}
	if got != version.DeployerVersion {
		return fmt.Errorf("version mismatch: deployer's deployer API version %s is incompatible with app' deployer API version %s.", version.DeployerVersion, got)
	}
	return nil
}
