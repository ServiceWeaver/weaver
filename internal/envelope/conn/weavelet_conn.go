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
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"runtime/pprof"
	"time"

	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/version"
)

// WeaveletHandler handles messages from the envelope. A handler should not
// block and should not perform RPCs over the pipe. Values passed to the
// handlers are only valid for the duration of the handler's execution.
type WeaveletHandler interface {
	// TODO(mwhittaker): Add context.Context to these methods?

	// GetLoad returns a load report.
	GetLoad(*protos.GetLoadRequest) (*protos.GetLoadReply, error)

	// UpdateComponents updates the set of components the weavelet should be
	// running. Currently, the set of components only increases over time.
	UpdateComponents(*protos.UpdateComponentsRequest) (*protos.UpdateComponentsReply, error)

	// UpdateRoutingInfo updates a component's routing information.
	UpdateRoutingInfo(*protos.UpdateRoutingInfoRequest) (*protos.UpdateRoutingInfoReply, error)
}

// WeaveletConn is the weavelet side of the connection between a weavelet and
// an envelope. For more information, refer to runtime/protos/runtime.proto and
// https://serviceweaver.dev/blog/deployers.html.
type WeaveletConn struct {
	handler WeaveletHandler
	conn    conn
	einfo   *protos.EnvelopeInfo
	winfo   *protos.WeaveletInfo
	lis     net.Listener // internal network listener for the weavelet
	metrics metrics.Exporter
}

// NewWeaveletConn returns a connection to an envelope. The connection sends
// messages to and receives messages from the envelope using r and w. Note that
// all RPCs will block until [Serve] is called.
//
// TODO(mwhittaker): Pass in a context.Context?
func NewWeaveletConn(r io.ReadCloser, w io.WriteCloser, h WeaveletHandler) (*WeaveletConn, error) {
	wc := &WeaveletConn{
		handler: h,
		conn:    conn{name: "weavelet", reader: r, writer: w},
	}

	// Perform the handshake. First, receive EnvelopeInfo.
	msg := &protos.EnvelopeMsg{}
	if err := wc.conn.recv(msg); err != nil {
		wc.conn.cleanup(err)
		return nil, err
	}
	wc.einfo = msg.EnvelopeInfo
	if wc.einfo == nil {
		err := fmt.Errorf("expected EnvelopeInfo, got %v", msg)
		wc.conn.cleanup(err)
		return nil, err
	}
	if err := runtime.CheckEnvelopeInfo(wc.einfo); err != nil {
		wc.conn.cleanup(err)
		return nil, err
	}

	// Second, send WeaveletInfo.
	lis, err := listen(wc.einfo)
	if err != nil {
		wc.conn.cleanup(err)
		return nil, err
	}
	wc.lis = lis
	dialAddr := fmt.Sprintf("tcp://%s", lis.Addr().String())
	if wc.einfo.Mtls {
		dialAddr = fmt.Sprintf("mtls://%s", dialAddr)
	}
	wc.winfo = &protos.WeaveletInfo{
		DialAddr: dialAddr,
		Pid:      int64(os.Getpid()),
		Version: &protos.SemVer{
			Major: version.Major,
			Minor: version.Minor,
			Patch: version.Patch,
		},
	}
	if err := wc.conn.send(&protos.WeaveletMsg{WeaveletInfo: wc.winfo}); err != nil {
		return nil, err
	}
	return wc, nil
}

// Serve accepts RPC requests from the envelope. Requests are handled serially
// in the order they are received.
func (w *WeaveletConn) Serve() error {
	msg := &protos.EnvelopeMsg{}
	for {
		if err := w.conn.recv(msg); err != nil {
			return err
		}
		if err := w.handleMessage(msg); err != nil {
			return err
		}
	}
}

// EnvelopeInfo returns the EnvelopeInfo received from the envelope.
func (w *WeaveletConn) EnvelopeInfo() *protos.EnvelopeInfo {
	return w.einfo
}

// WeaveletInfo returns the WeaveletInfo sent to the envelope.
func (w *WeaveletConn) WeaveletInfo() *protos.WeaveletInfo {
	return w.winfo
}

// Listener returns the internal network listener for the weavelet.
func (w *WeaveletConn) Listener() net.Listener {
	return w.lis
}

// handleMessage handles all RPC requests initiated by the envelope. Note that
// this method doesn't handle RPC replies from the envelope.
func (w *WeaveletConn) handleMessage(msg *protos.EnvelopeMsg) error {
	errstring := func(err error) string {
		if err == nil {
			return ""
		}
		return err.Error()
	}

	switch {
	case msg.GetMetricsRequest != nil:
		// Inject Service Weaver specific labels.
		update := w.metrics.Export()
		for _, def := range update.Defs {
			if def.Labels == nil {
				def.Labels = map[string]string{}
			}
			def.Labels["serviceweaver_app"] = w.einfo.App
			def.Labels["serviceweaver_version"] = w.einfo.DeploymentId
			def.Labels["serviceweaver_node"] = w.einfo.Id
		}
		return w.conn.send(&protos.WeaveletMsg{
			Id:              -msg.Id,
			GetMetricsReply: &protos.GetMetricsReply{Update: update},
		})
	case msg.GetHealthRequest != nil:
		return w.conn.send(&protos.WeaveletMsg{
			Id:             -msg.Id,
			GetHealthReply: &protos.GetHealthReply{Status: protos.HealthStatus_HEALTHY},
		})
	case msg.GetLoadRequest != nil:
		reply, err := w.handler.GetLoad(msg.GetLoadRequest)
		return w.conn.send(&protos.WeaveletMsg{
			Id:           -msg.Id,
			Error:        errstring(err),
			GetLoadReply: reply,
		})
	case msg.GetProfileRequest != nil:
		// This is a blocking call, and therefore we process it in a separate
		// goroutine. Note that this will cause profiling requests to be
		// processed out-of-order w.r.t. other messages.
		id := msg.Id
		req := protomsg.Clone(msg.GetProfileRequest)
		go func() {
			data, err := Profile(req)
			// Reply with profile data.
			//nolint:errcheck //errMsg will be returned on next send
			w.conn.send(&protos.WeaveletMsg{
				Id:              -id,
				Error:           errstring(err),
				GetProfileReply: &protos.GetProfileReply{Data: data},
			})
		}()
		return nil
	case msg.UpdateComponentsRequest != nil:
		reply, err := w.handler.UpdateComponents(msg.UpdateComponentsRequest)
		return w.conn.send(&protos.WeaveletMsg{
			Id:                    -msg.Id,
			Error:                 errstring(err),
			UpdateComponentsReply: reply,
		})
	case msg.UpdateRoutingInfoRequest != nil:
		reply, err := w.handler.UpdateRoutingInfo(msg.UpdateRoutingInfoRequest)
		return w.conn.send(&protos.WeaveletMsg{
			Id:                     -msg.Id,
			Error:                  errstring(err),
			UpdateRoutingInfoReply: reply,
		})
	default:
		err := fmt.Errorf("weavelet_conn: unexpected message %+v", msg)
		w.conn.cleanup(err)
		return err
	}
}

// ActivateComponentRPC ensures that the provided component is running
// somewhere. A call to ActivateComponentRPC also implicitly signals that a
// weavelet is interested in receiving routing info for the component.
func (w *WeaveletConn) ActivateComponentRPC(req *protos.ActivateComponentRequest) error {
	reply, err := w.rpc(&protos.WeaveletMsg{ActivateComponentRequest: req})
	if err != nil {
		return err
	}
	if reply.ActivateComponentReply == nil {
		return fmt.Errorf("nil ActivateComponentReply received from envelope")
	}
	return nil
}

// GetListenerAddressRPC returns the address the weavelet should listen on for
// a particular listener.
func (w *WeaveletConn) GetListenerAddressRPC(req *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
	reply, err := w.rpc(&protos.WeaveletMsg{GetListenerAddressRequest: req})
	if err != nil {
		return nil, err
	}
	if reply.GetListenerAddressReply == nil {
		return nil, fmt.Errorf("nil GetListenerAddressReply received from envelope")
	}
	return reply.GetListenerAddressReply, nil
}

// ExportListenerRPC exports the provided listener.
func (w *WeaveletConn) ExportListenerRPC(req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	reply, err := w.rpc(&protos.WeaveletMsg{ExportListenerRequest: req})
	if err != nil {
		return nil, err
	}
	if reply.ExportListenerReply == nil {
		return nil, fmt.Errorf("nil ExportListenerReply received from envelope")
	}
	return reply.ExportListenerReply, nil
}

// GetSelfCertificateRPC returns the certificate and the private key the
// weavelet should use for network connection establishment.
func (w *WeaveletConn) GetSelfCertificateRPC(req *protos.GetSelfCertificateRequest) (*protos.GetSelfCertificateReply, error) {
	reply, err := w.rpc(&protos.WeaveletMsg{GetSelfCertificateRequest: req})
	if err != nil {
		return nil, err
	}
	if reply.GetSelfCertificateReply == nil {
		return nil, fmt.Errorf("nil GetSelfCertificateReply received from envelope")
	}
	return reply.GetSelfCertificateReply, nil
}

// VerifyClientCertificateRPC verifies the identity of a client that is
// attempting to connect to the weavelet.
func (w *WeaveletConn) VerifyClientCertificateRPC(req *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error) {
	reply, err := w.rpc(&protos.WeaveletMsg{VerifyClientCertificateRequest: req})
	if err != nil {
		return nil, err
	}
	if reply.VerifyClientCertificateReply == nil {
		return nil, fmt.Errorf("nil VerifyClientCertificateReply received from envelope")
	}
	return reply.VerifyClientCertificateReply, nil
}

// VerifyServerCertificateRPC verifies the identity of the server the weavelet
// is attempting to connect to.
func (w *WeaveletConn) VerifyServerCertificateRPC(req *protos.VerifyServerCertificateRequest) error {
	reply, err := w.rpc(&protos.WeaveletMsg{VerifyServerCertificateRequest: req})
	if err != nil {
		return err
	}
	if reply.VerifyServerCertificateReply == nil {
		return fmt.Errorf("nil VerifyServerCertificateReply received from envelope")
	}
	return nil
}

func (w *WeaveletConn) rpc(request *protos.WeaveletMsg) (*protos.EnvelopeMsg, error) {
	response, err := w.conn.doBlockingRPC(request)
	if err != nil {
		err := fmt.Errorf("connection to envelope broken: %w", err)
		w.conn.cleanup(err)
		return nil, err
	}
	msg, ok := response.(*protos.EnvelopeMsg)
	if !ok {
		return nil, fmt.Errorf("envelope response has wrong type %T", response)
	}
	if msg.Error != "" {
		return nil, fmt.Errorf(msg.Error)
	}
	return msg, nil
}

// SendLogEntry sends a log entry to the envelope, without waiting for a reply.
func (w *WeaveletConn) SendLogEntry(entry *protos.LogEntry) error {
	return w.conn.send(&protos.WeaveletMsg{LogEntry: entry})
}

// SendTraceSpans sends a set of trace spans to the envelope, without waiting
// for a reply.
func (w *WeaveletConn) SendTraceSpans(spans *protos.TraceSpans) error {
	return w.conn.send(&protos.WeaveletMsg{TraceSpans: spans})
}

// Profile collects profiles for the weavelet.
func Profile(req *protos.GetProfileRequest) ([]byte, error) {
	var buf bytes.Buffer
	switch req.ProfileType {
	case protos.ProfileType_Heap:
		if err := pprof.WriteHeapProfile(&buf); err != nil {
			return nil, err
		}
	case protos.ProfileType_CPU:
		if req.CpuDurationNs == 0 {
			return nil, fmt.Errorf("invalid zero duration for the CPU profile collection")
		}
		dur := time.Duration(req.CpuDurationNs) * time.Nanosecond
		if err := pprof.StartCPUProfile(&buf); err != nil {
			return nil, err
		}
		time.Sleep(dur)
		pprof.StopCPUProfile()
	default:
		return nil, fmt.Errorf("unspecified profile collection type")
	}
	return buf.Bytes(), nil
}

func listen(info *protos.EnvelopeInfo) (net.Listener, error) {
	// Pick a hostname to listen on.
	host := "localhost"
	if !info.SingleMachine {
		// TODO(mwhittaker): Right now, we resolve our hostname to get a
		// dialable IP address. Double check that this always works.
		var err error
		host, err = os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("error getting local hostname: %w", err)
		}
	}

	// Create the listener
	return net.Listen("tcp", fmt.Sprintf("%s:%d", host, info.InternalPort))
}
