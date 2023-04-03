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
	info    *protos.EnvelopeInfo
	lis     net.Listener // internal network listener for the weavelet
	metrics metrics.Exporter
}

// NewWeaveletConn returns a connection to an envelope. The connection sends
// messages to and receives messages from the envelope using r and w. Note that
// all RPCs will block until [Serve] is called.
//
// TODO(mwhittaker): Pass in a context.Context?
func NewWeaveletConn(r io.ReadCloser, w io.WriteCloser, h WeaveletHandler) (*WeaveletConn, error) {
	d := &WeaveletConn{
		handler: h,
		conn:    conn{name: "weavelet", reader: r, writer: w},
	}

	// Perform the handshake. First, receive EnvelopeInfo.
	msg := &protos.EnvelopeMsg{}
	if err := d.conn.recv(msg); err != nil {
		d.conn.cleanup(err)
		return nil, err
	}
	d.info = msg.EnvelopeInfo
	if d.info == nil {
		err := fmt.Errorf("expected EnvelopeInfo, got %v", msg)
		d.conn.cleanup(err)
		return nil, err
	}
	if err := runtime.CheckEnvelopeInfo(d.info); err != nil {
		d.conn.cleanup(err)
		return nil, err
	}

	// Second, send WeaveletInfo.
	lis, err := listen(d.info)
	if err != nil {
		d.conn.cleanup(err)
		return nil, err
	}
	d.lis = lis
	dialAddr := fmt.Sprintf("tcp://%s", lis.Addr().String())
	info := &protos.WeaveletInfo{
		DialAddr: dialAddr,
		Pid:      int64(os.Getpid()),
		Version: &protos.SemVer{
			Major: runtime.Major,
			Minor: runtime.Minor,
			Patch: runtime.Patch,
		},
	}
	if err := d.conn.send(&protos.WeaveletMsg{WeaveletInfo: info}); err != nil {
		return nil, err
	}
	return d, nil
}

// Serve accepts RPC requests from the envelope. Requests are handled serially
// in the order they are received.
func (d *WeaveletConn) Serve() error {
	msg := &protos.EnvelopeMsg{}
	for {
		if err := d.conn.recv(msg); err != nil {
			return err
		}
		if err := d.handleMessage(msg); err != nil {
			return err
		}
	}
}

// EnvelopeInfo returns the EnvelopeInfo received from the envelope.
func (d *WeaveletConn) EnvelopeInfo() *protos.EnvelopeInfo {
	return d.info
}

// Listener returns the internal network listener for the weavelet.
func (d *WeaveletConn) Listener() net.Listener {
	return d.lis
}

// handleMessage handles all RPC requests initiated by the envelope. Note that
// this method doesn't handle RPC replies from the envelope.
func (d *WeaveletConn) handleMessage(msg *protos.EnvelopeMsg) error {
	errstring := func(err error) string {
		if err == nil {
			return ""
		}
		return err.Error()
	}

	switch {
	case msg.GetMetricsRequest != nil:
		// Inject Service Weaver specific labels.
		update := d.metrics.Export()
		for _, def := range update.Defs {
			if def.Labels == nil {
				def.Labels = map[string]string{}
			}
			def.Labels["serviceweaver_app"] = d.info.App
			def.Labels["serviceweaver_version"] = d.info.DeploymentId
			def.Labels["serviceweaver_node"] = d.info.Id
		}
		return d.conn.send(&protos.WeaveletMsg{
			Id:              -msg.Id,
			GetMetricsReply: &protos.GetMetricsReply{Update: update},
		})
	case msg.GetHealthRequest != nil:
		return d.conn.send(&protos.WeaveletMsg{
			Id:             -msg.Id,
			GetHealthReply: &protos.GetHealthReply{Status: protos.HealthStatus_HEALTHY},
		})
	case msg.GetLoadRequest != nil:
		reply, err := d.handler.GetLoad(msg.GetLoadRequest)
		return d.conn.send(&protos.WeaveletMsg{
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
			d.conn.send(&protos.WeaveletMsg{
				Id:              -id,
				Error:           errstring(err),
				GetProfileReply: &protos.GetProfileReply{Data: data},
			})
		}()
		return nil
	case msg.UpdateComponentsRequest != nil:
		reply, err := d.handler.UpdateComponents(msg.UpdateComponentsRequest)
		return d.conn.send(&protos.WeaveletMsg{
			Id:                    -msg.Id,
			Error:                 errstring(err),
			UpdateComponentsReply: reply,
		})
	case msg.UpdateRoutingInfoRequest != nil:
		reply, err := d.handler.UpdateRoutingInfo(msg.UpdateRoutingInfoRequest)
		return d.conn.send(&protos.WeaveletMsg{
			Id:                     -msg.Id,
			Error:                  errstring(err),
			UpdateRoutingInfoReply: reply,
		})
	default:
		err := fmt.Errorf("weavelet_conn: unexpected message %+v", msg)
		d.conn.cleanup(err)
		return err
	}
}

// ActivateComponentRPC ensures that the provided component is running
// somewhere. A call to ActivateComponentRPC also implicitly signals that a
// weavelet is interested in receiving routing info for the component.
func (d *WeaveletConn) ActivateComponentRPC(req *protos.ActivateComponentRequest) error {
	reply, err := d.rpc(&protos.WeaveletMsg{ActivateComponentRequest: req})
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
func (d *WeaveletConn) GetListenerAddressRPC(req *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
	reply, err := d.rpc(&protos.WeaveletMsg{GetListenerAddressRequest: req})
	if err != nil {
		return nil, err
	}
	if reply.GetListenerAddressReply == nil {
		return nil, fmt.Errorf("nil GetListenerAddressReply received from envelope")
	}
	return reply.GetListenerAddressReply, nil
}

// ExportListenerRPC exports the provided listener.
func (d *WeaveletConn) ExportListenerRPC(req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	reply, err := d.rpc(&protos.WeaveletMsg{ExportListenerRequest: req})
	if err != nil {
		return nil, err
	}
	if reply.ExportListenerReply == nil {
		return nil, fmt.Errorf("nil ExportListenerReply received from envelope")
	}
	return reply.ExportListenerReply, nil
}

func (d *WeaveletConn) rpc(request *protos.WeaveletMsg) (*protos.EnvelopeMsg, error) {
	response, err := d.conn.doBlockingRPC(request)
	if err != nil {
		err := fmt.Errorf("connection to envelope broken: %w", err)
		d.conn.cleanup(err)
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
func (d *WeaveletConn) SendLogEntry(entry *protos.LogEntry) error {
	return d.conn.send(&protos.WeaveletMsg{LogEntry: entry})
}

// SendTraceSpans sends a set of trace spans to the envelope, without waiting
// for a reply.
func (d *WeaveletConn) SendTraceSpans(spans *protos.TraceSpans) error {
	return d.conn.send(&protos.WeaveletMsg{TraceSpans: spans})
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
	return net.Listen("tcp", fmt.Sprintf("%s:0", host))
}
