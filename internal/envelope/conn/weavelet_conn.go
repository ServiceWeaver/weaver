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
	"net"

	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/version"
)

// WeaveletConn is the weavelet side of the connection between a weavelet and
// an envelope. For more information, refer to runtime/protos/runtime.proto and
// https://serviceweaver.dev/blog/deployers.html.
type WeaveletConn struct {
	conn  conn
	einfo *protos.EnvelopeInfo
	winfo *protos.WeaveletInfo
	lis   net.Listener // internal network listener for the weavelet
}

// NewWeaveletConn returns a connection to an envelope. The connection sends
// messages to and receives messages from the envelope using r and w. Note that
// all RPCs will block until [Serve] is called.
//
// TODO(mwhittaker): Pass in a context.Context?
func NewWeaveletConn(r io.ReadCloser, w io.WriteCloser) (*WeaveletConn, error) {
	wc := &WeaveletConn{
		conn: conn{name: "weavelet", reader: r, writer: w},
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
	lis, err := net.Listen("tcp", wc.einfo.InternalAddress)
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
		Version: &protos.SemVer{
			Major: version.DeployerMajor,
			Minor: version.DeployerMinor,
			Patch: 0,
		},
	}
	if err := wc.conn.send(&protos.WeaveletMsg{WeaveletInfo: wc.winfo}); err != nil {
		return nil, err
	}
	return wc, nil
}

// Serve handles RPC responses from the envelope.
func (w *WeaveletConn) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		w.conn.cleanup(ctx.Err())
	}()

	msg := &protos.EnvelopeMsg{}
	if err := w.conn.recv(msg); err != nil {
		return err
	}
	// We do not support any requests initiated by the envelope.
	err := fmt.Errorf("weavelet_conn: unexpected message %+v", msg)
	w.conn.cleanup(err)
	return err
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
