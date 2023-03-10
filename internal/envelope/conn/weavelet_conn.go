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
	"runtime/pprof"
	"time"

	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// WeaveletConn is the weavelet side of the connection between a weavelet
// and its envelope. It communicates with the envelope over a pair of pipes.
type WeaveletConn struct {
	conn    conn
	wlet    *protos.WeaveletInfo
	metrics metrics.Exporter
}

// NewWeaveletConn creates the weavelet side of the connection between a
// weavelet and its envelope. The connection uses (r,w) to carry messages.
// Synthesized high-level events are passed to h.
//
// NewWeaveletConn blocks until it receives a protos.Weavelet from the
// envelope.
func NewWeaveletConn(r io.ReadCloser, w io.WriteCloser) (*WeaveletConn, error) {
	d := &WeaveletConn{conn: conn{name: "weavelet", reader: r, writer: w}}

	// Block until a weavelet is received.
	msg := &protos.EnvelopeMsg{}
	if err := d.conn.recv(msg); err != nil {
		d.conn.cleanup(err)
		return nil, err
	}
	d.wlet = msg.WeaveletInfo
	if d.wlet == nil {
		err := fmt.Errorf("expected WeaveletInfo, got %v", msg)
		d.conn.cleanup(err)
		return nil, err
	}
	if err := runtime.CheckWeaveletInfo(d.wlet); err != nil {
		d.conn.cleanup(err)
		return nil, err
	}
	return d, nil
}

// Run interacts with the peer. Messages that are received are
// handled as an ordered sequence.
func (d *WeaveletConn) Run() error {
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

// Weavelet returns the protos.Weavelet for this weavelet.
func (d *WeaveletConn) Weavelet() *protos.WeaveletInfo {
	return d.wlet
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
			def.Labels["serviceweaver_app"] = d.wlet.App
			def.Labels["serviceweaver_version"] = d.wlet.DeploymentId
			def.Labels["serviceweaver_node"] = d.wlet.Id
		}
		return d.send(&protos.WeaveletMsg{Id: -msg.Id, Metrics: update})
	case msg.SendHealthStatus:
		return d.send(&protos.WeaveletMsg{Id: -msg.Id, HealthReport: &protos.HealthReport{Status: protos.HealthStatus_HEALTHY}})
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
				d.send(&protos.WeaveletMsg{Id: -id, Error: err.Error()})
				return
			}
			// Reply with profile data.
			//nolint:errcheck // error will be returned on next send
			d.send(&protos.WeaveletMsg{Id: -id, Profile: &protos.Profile{
				AppName:   req.AppName,
				VersionId: req.VersionId,
				Data:      data,
			}})
		}()
		return nil
	default:
		err := fmt.Errorf("unexpected message %v", msg)
		d.conn.cleanup(err)
		return err
	}
}

// StartComponentRPC requests the envelope to start the given component.
func (d *WeaveletConn) StartComponentRPC(componentToStart *protos.ComponentToStart) error {
	_, err := d.rpc(&protos.WeaveletMsg{ComponentToStart: componentToStart})
	return err
}

// RegisterReplicaRPC requests the envelope to register the given replica.
func (d *WeaveletConn) RegisterReplicaRPC(replica *protos.ReplicaToRegister) error {
	_, err := d.rpc(&protos.WeaveletMsg{ReplicaToRegister: replica})
	return err
}

// ReportLoadRPC reports the given load to the envelope.
func (d *WeaveletConn) ReportLoadRPC(load *protos.WeaveletLoadReport) error {
	_, err := d.rpc(&protos.WeaveletMsg{LoadReport: load})
	return err
}

// GetRoutingInfoRPC requests the envelope to send us the latest routing info.
func (d *WeaveletConn) GetRoutingInfoRPC(info *protos.GetRoutingInfo) (*protos.RoutingInfo, error) {
	reply, err := d.rpc(&protos.WeaveletMsg{GetRoutingInfo: info})
	if err != nil {
		return nil, err
	}
	if reply.RoutingInfo == nil {
		return nil, fmt.Errorf("nil routing info received from envelope")
	}
	return reply.RoutingInfo, nil
}

// GetComponentsToStartRPC is a blocking call to the envelope to send us a list
// of components to start.
func (d *WeaveletConn) GetComponentsToStartRPC(info *protos.GetComponentsToStart) (*protos.ComponentsToStart, error) {
	reply, err := d.rpc(&protos.WeaveletMsg{GetComponentsToStart: info})
	if err != nil {
		return nil, err
	}
	if reply.ComponentsToStart == nil {
		return nil, fmt.Errorf("nil components to start reply received from envelope")
	}
	return reply.ComponentsToStart, nil
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
	response, err := d.conn.rpc(request)
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

func (d *WeaveletConn) send(msg *protos.WeaveletMsg) error {
	if err := d.conn.send(msg); err != nil {
		d.conn.cleanup(err)
		return err
	}
	return nil
}

// SendLogEntry sends a log entry to the envelope, without waiting for a reply.
func (d *WeaveletConn) SendLogEntry(entry *protos.LogEntry) {
	//nolint:errcheck // error will be returned on next send
	d.send(&protos.WeaveletMsg{LogEntry: entry})
}

// SendTraceSpans sends a set of trace spans to the envelope, without waiting
// for a reply.
func (d *WeaveletConn) SendTraceSpans(spans *protos.Spans) error {
	return d.send(&protos.WeaveletMsg{TraceSpans: spans})
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
