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

// WeaveletHandler implements the weavelet side processing of messages exchanged
// with the corresponding envelope.
//
// The handlers should not block and should not communicate over the pipe.
type WeaveletHandler interface {
	// CollectLoad returns the latest load information at the weavelet.
	CollectLoad() (*protos.WeaveletLoadReport, error)

	// UpdateComponents is called with the latest set of components to run.
	UpdateComponents(*protos.ComponentsToStart) error

	// UpdateRoutingInfo is called with the latest routing info for a
	// component.
	UpdateRoutingInfo(*protos.RoutingInfo) error
}

// WeaveletConn is the weavelet side of the connection between a weavelet
// and its envelope. It communicates with the envelope over a pair of pipes.
type WeaveletConn struct {
	handler WeaveletHandler
	conn    conn
	info    *protos.WeaveletSetupInfo
	lis     net.Listener // internal network listener for the weavelet
	metrics metrics.Exporter
}

// NewWeaveletConn creates the weavelet side of the connection between a
// weavelet and its envelope. The connection uses (r,w) to carry messages.
// Synthesized high-level events are passed to h.
//
// NewWeaveletConn blocks until it receives a protos.Weavelet from the
// envelope.
func NewWeaveletConn(r io.ReadCloser, w io.WriteCloser, h WeaveletHandler) (*WeaveletConn, error) {
	d := &WeaveletConn{
		handler: h,
		conn:    conn{name: "weavelet", reader: r, writer: w},
	}

	// Block until the weavelet information is received.
	msg := &protos.EnvelopeMsg{}
	if err := d.conn.recv(msg); err != nil {
		d.conn.cleanup(err)
		return nil, err
	}
	d.info = msg.WeaveletSetupInfo
	if d.info == nil {
		err := fmt.Errorf("expected WeaveletSetupInfo, got %v", msg)
		d.conn.cleanup(err)
		return nil, err
	}
	if err := runtime.CheckWeaveletSetupInfo(d.info); err != nil {
		d.conn.cleanup(err)
		return nil, err
	}

	// Reply to the envelope with weavelet's runtime information.
	lis, err := listen(d.info)
	if err != nil {
		d.conn.cleanup(err)
		return nil, err
	}
	d.lis = lis
	dialAddr := fmt.Sprintf("tcp://%s", lis.Addr().String())
	info := &protos.WeaveletInfo{DialAddr: dialAddr, Pid: int64(os.Getpid())}
	if err := d.conn.send(&protos.WeaveletMsg{WeaveletInfo: info}); err != nil {
		return nil, err
	}
	return d, nil
}

// Serve accepts incoming messages from the envelope. Messages that are received
// are handled as an ordered sequence.
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

// Bootstrap returns the setup information for the weavelet.
func (d *WeaveletConn) WeaveletSetupInfo() *protos.WeaveletSetupInfo {
	return d.info
}

// Listener returns the internal network listener for the weavelet.
func (d *WeaveletConn) Listener() net.Listener {
	return d.lis
}

// handleMessage handles all messages initiated by the envelope. Note that
// this method doesn't handle RPC reply messages sent over by the envelope.
func (d *WeaveletConn) handleMessage(msg *protos.EnvelopeMsg) error {
	switch {
	case msg.SendMetrics:
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
		return d.conn.send(&protos.WeaveletMsg{Id: -msg.Id, Metrics: update})
	case msg.SendHealthStatus:
		return d.conn.send(&protos.WeaveletMsg{Id: -msg.Id, HealthReport: &protos.HealthReport{Status: protos.HealthStatus_HEALTHY}})
	case msg.SendLoadInfo:
		id := msg.Id
		load, err := d.handler.CollectLoad()
		if err != nil {
			return d.conn.send(&protos.WeaveletMsg{Id: -id, Error: err.Error()})
		}
		return d.conn.send(&protos.WeaveletMsg{Id: -id, LoadReport: load})
	case msg.RunProfiling != nil:
		// This is a blocking call, and therefore we process it in a separate
		// goroutine. Note that this will cause profiling requests to be
		// processed out-of-order w.r.t. other messages.
		id := msg.Id
		req := protomsg.Clone(msg.RunProfiling)
		go func() {
			data, err := Profile(req)
			if err != nil {
				// Reply with error.
				//nolint:errcheck // error will be returned on next send
				d.conn.send(&protos.WeaveletMsg{Id: -id, Error: err.Error()})
				return
			}
			// Reply with profile data.
			//nolint:errcheck // error will be returned on next send
			d.conn.send(&protos.WeaveletMsg{Id: -id, Profile: &protos.Profile{
				AppName:   req.AppName,
				VersionId: req.VersionId,
				Data:      data,
			}})
		}()
		return nil
	case msg.ComponentsToStart != nil:
		id := msg.Id
		err := d.handler.UpdateComponents(msg.ComponentsToStart)
		if err != nil {
			return d.conn.send(&protos.WeaveletMsg{Id: -id, Error: err.Error()})
		}
		return d.conn.send(&protos.WeaveletMsg{Id: -id})
	case msg.RoutingInfo != nil:
		id := msg.Id
		err := d.handler.UpdateRoutingInfo(msg.RoutingInfo)
		if err != nil {
			return d.conn.send(&protos.WeaveletMsg{Id: -id, Error: err.Error()})
		}
		return d.conn.send(&protos.WeaveletMsg{Id: -id})
	default:
		err := fmt.Errorf("weavelet_conn: unexpected message %+v", msg)
		d.conn.cleanup(err)
		return err
	}
}

// StartComponentRPC requests the envelope to start the given component.
func (d *WeaveletConn) StartComponentRPC(componentToStart *protos.ComponentToStart) error {
	_, err := d.rpc(&protos.WeaveletMsg{ComponentToStart: componentToStart})
	return err
}

func (d *WeaveletConn) GetAddressRPC(req *protos.GetAddressRequest) (*protos.GetAddressReply, error) {
	reply, err := d.rpc(&protos.WeaveletMsg{GetAddressRequest: req})
	if err != nil {
		return nil, err
	}
	if reply.GetAddressReply == nil {
		return nil, fmt.Errorf("nil GetAddressReply recieved from envelope")
	}
	return reply.GetAddressReply, nil
}

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
func (d *WeaveletConn) SendLogEntry(entry *protos.LogEntry) {
	//nolint:errcheck // error will be returned on next send
	d.conn.send(&protos.WeaveletMsg{LogEntry: entry})
}

// SendTraceSpans sends a set of trace spans to the envelope, without waiting
// for a reply.
func (d *WeaveletConn) SendTraceSpans(spans *protos.Spans) error {
	return d.conn.send(&protos.WeaveletMsg{TraceSpans: spans})
}

// Profile collects profiles for the weavelet.
func Profile(req *protos.RunProfiling) ([]byte, error) {
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

func listen(info *protos.WeaveletSetupInfo) (net.Listener, error) {
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
