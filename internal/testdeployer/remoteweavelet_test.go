// Copyright 2023 Google LLC
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

package testdeployer

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/internal/weaver"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
)

// TODO(mwhittaker):
//
// - Kill the remote weavelet at the end of every test.
// - Create a weavelet and then break the connection.
// - Add a Wait method to RemoteWeavelet to wait for shutdown.
// - Hook up two weavelets then cancel one of them.
// - Update routing info with bad routing info.
// - Component with a failing Init method.
// - Component with a blocking Init method.

// deployer is a test deployer that spawns one weavelet which hosts all
// components.
type deployer struct {
	t                *testing.T             // underlying unit test
	ctx              context.Context        // context used to spawn weavelet
	cancel           context.CancelFunc     // cancels weavelet
	toWeaveletReader *os.File               // reader end of pipe to weavelet
	toWeaveletWriter *os.File               // writer end of pipe to weavelet
	toEnvelopeReader *os.File               // reader end of pipe to envelope
	toEnvelopeWriter *os.File               // writer end of pipe to envelope
	env              *conn.EnvelopeConn     // envelope
	wlet             *weaver.RemoteWeavelet // weavelet
	logger           *logging.TestLogger    // logger

	// A unit test can override the following envelope methods to do things
	// like inject errors or return invalid values.
	mu                 sync.Mutex
	activateComponent  func(context.Context, *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error)
	getListenerAddress func(context.Context, *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error)
	exportListener     func(context.Context, *protos.ExportListenerRequest) (*protos.ExportListenerReply, error)
}

// ActivateComponent implements the EnvelopeHandler interface.
func (d *deployer) ActivateComponent(ctx context.Context, req *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.activateComponent != nil {
		return d.activateComponent(ctx, req)
	}

	// Because there is only one weavelet which hosts all components, we route
	// all requests locally.
	routing := &protos.UpdateRoutingInfoRequest{
		RoutingInfo: &protos.RoutingInfo{
			Component: req.Component,
			Local:     true,
		},
	}
	if _, err := d.wlet.UpdateRoutingInfo(routing); err != nil {
		return nil, err
	}

	// Start the requested component.
	components := &protos.UpdateComponentsRequest{
		Components: []string{req.Component},
	}
	if _, err := d.wlet.UpdateComponents(components); err != nil {
		return nil, err
	}

	return &protos.ActivateComponentReply{}, nil
}

// GetListenerAddress implements the EnvelopeHandler interface.
func (d *deployer) GetListenerAddress(ctx context.Context, req *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.getListenerAddress != nil {
		return d.getListenerAddress(ctx, req)
	}
	return &protos.GetListenerAddressReply{Address: ":0"}, nil
}

// ExportListenerAddress implements the EnvelopeHandler interface.
func (d *deployer) ExportListener(ctx context.Context, req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.exportListener != nil {
		return d.exportListener(ctx, req)
	}
	return &protos.ExportListenerReply{}, nil
}

// HandleLogEntry implements the EnvelopeHandler interface.
func (d *deployer) HandleLogEntry(_ context.Context, entry *protos.LogEntry) error {
	d.logger.Log(entry)
	return nil
}

// HandleTraceSpans implements the EnvelopeHandler interface.
func (d *deployer) HandleTraceSpans(context.Context, *protos.TraceSpans) error {
	return nil
}

// GetSelfCertificate implements the EnvelopeHandler interface.
func (d *deployer) GetSelfCertificate(context.Context, *protos.GetSelfCertificateRequest) (*protos.GetSelfCertificateReply, error) {
	d.t.Fatal("unimplemented")
	return nil, nil
}

// VerifyClientCertificate implements the EnvelopeHandler interface.
func (d *deployer) VerifyClientCertificate(context.Context, *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error) {
	d.t.Fatal("unimplemented")
	return nil, nil
}

// VerifyServerCertificate implements the EnvelopeHandler interface.
func (d *deployer) VerifyServerCertificate(context.Context, *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error) {
	d.t.Fatal("unimplemented")
	return nil, nil
}

// deploy creates a new test deployer with a single spawned weavelet.
func deploy(t *testing.T, ctx context.Context) *deployer {
	return deployWithInfo(t, ctx, &protos.EnvelopeInfo{
		App:             "remoteweavelet_test.go",
		DeploymentId:    fmt.Sprint(os.Getpid()),
		Id:              uuid.New().String(),
		InternalAddress: "localhost:0",
	})
}

// deploy creates a new test deployer with a single spawned weavelet. The
// deployer relays the provided EnvelopeInfo to the weavelet.
func deployWithInfo(t *testing.T, ctx context.Context, info *protos.EnvelopeInfo) *deployer {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)

	// Create pipes to the weavelet.
	toWeaveletReader, toWeaveletWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	toEnvelopeReader, toEnvelopeWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// TODO(mwhittaker): Make sure to close these. This is a bit tricky because
	// if we close a pipe to the weavelet, it will os.Exit.

	// conn.NewEnvelopeConn blocks performing a handshake with the weavelet, so
	// we have to run it in a separate goroutine.
	errs := make(chan error)
	var env *conn.EnvelopeConn
	go func() {
		var err error
		env, err = conn.NewEnvelopeConn(ctx, toEnvelopeReader, toWeaveletWriter, info)
		errs <- err
	}()

	// Create the weavelet.
	wlet, err := weaver.NewRemoteWeavelet(
		ctx,
		codegen.Registered(),
		runtime.Bootstrap{ToWeaveletFile: toWeaveletReader, ToEnvelopeFile: toEnvelopeWriter},
		weaver.RemoteWeaveletOptions{},
	)
	if err != nil {
		t.Fatalf("NewRemoteWeavelet: %v", err)
	}

	// Wait for the EnvelopeConn to finish the handshake.
	if err := <-errs; err != nil {
		t.Fatalf("NewEnvelopeConn: %v", err)
	}

	d := &deployer{
		t:                t,
		ctx:              ctx,
		cancel:           cancel,
		toWeaveletReader: toWeaveletReader,
		toWeaveletWriter: toWeaveletWriter,
		toEnvelopeReader: toEnvelopeReader,
		toEnvelopeWriter: toEnvelopeWriter,
		env:              env,
		wlet:             wlet,
		logger:           logging.NewTestLogger(t, testing.Verbose()),
	}
	t.Cleanup(d.shutdown)
	return d
}

// shutdown shuts down a deployer and its weavelet.
func (d *deployer) shutdown() {
	d.cancel()
	// TODO(mwhittaker): Wait for the weavelet to exit.
}

// testComponents tests that the components spawned by d are working properly.
func testComponents(d *deployer) {
	d.t.Helper()
	const want = 42
	x, err := d.wlet.GetIntf(reflection.Type[a]())
	if err != nil {
		d.t.Fatal(err)
	}
	got, err := x.(a).A(d.ctx, want)
	if err != nil {
		d.t.Fatal(err)
	}
	if got != want {
		d.t.Fatalf("A(%d): got %d, want %d", want, got, want)
	}
}

func TestInvalidPipes(t *testing.T) {
	// Create an already closed pipe.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	r.Close()

	// Construct a remote weavelet, which should fail.
	if _, err := weaver.NewRemoteWeavelet(
		context.Background(),
		codegen.Registered(),
		runtime.Bootstrap{ToWeaveletFile: r, ToEnvelopeFile: w},
		weaver.RemoteWeaveletOptions{},
	); err == nil {
		t.Fatal("unexpected success")
	}
}

func TestInvalidHandshake(t *testing.T) {
	// Create pipes to the weavelet.
	r1, w1, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r1.Close()
	defer w1.Close()

	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	defer w2.Close()

	// When the weavelet is created, it performs a handshake with the envelope
	// and expects to receive an EnvelopeInfo, but instead, we send it garbage.
	var garbage [1000]byte
	if _, err := w1.Write(garbage[:]); err != nil {
		t.Fatal(err)
	}

	// Construct a remote weavelet, which should fail.
	if _, err := weaver.NewRemoteWeavelet(
		context.Background(),
		codegen.Registered(),
		runtime.Bootstrap{ToWeaveletFile: r1, ToEnvelopeFile: w2},
		weaver.RemoteWeaveletOptions{},
	); err == nil {
		t.Fatal("unexpected success")
	}
}

func TestLocalhostWeaveletAddress(t *testing.T) {
	// Start the weavelet with internal address "localhost:12345".
	d := deployWithInfo(t, context.Background(), &protos.EnvelopeInfo{
		App:             "remoteweavelet_test.go",
		DeploymentId:    fmt.Sprint(os.Getpid()),
		Id:              uuid.New().String(),
		InternalAddress: "localhost:12345",
	})
	got := d.env.WeaveletInfo().DialAddr
	const want = "tcp://127.0.0.1:12345"
	if got != want {
		t.Fatalf("DialAddr: got %q, want %q", got, want)
	}
}

func TestHostnameWeaveletAddress(t *testing.T) {
	// Start the weavelet with internal address "$HOSTNAME:12345".
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	ips, err := net.LookupIP(hostname)
	if err != nil {
		t.Fatalf("net.LookupIP(%q): %v", hostname, err)
	}
	if len(ips) == 0 {
		t.Fatalf("net.LookupIP(%q): no IPs", hostname)
	}

	d := deployWithInfo(t, context.Background(), &protos.EnvelopeInfo{
		App:             "remoteweavelet_test.go",
		DeploymentId:    fmt.Sprint(os.Getpid()),
		Id:              uuid.New().String(),
		InternalAddress: fmt.Sprintf("%s:12345", ips[0]),
	})
	got := d.env.WeaveletInfo().DialAddr
	want := fmt.Sprintf("tcp://%v:12345", ips[0])
	if got != want {
		t.Fatalf("DialAddr: got %q, want %q", got, want)
	}
}

func TestErrorFreeExecution(t *testing.T) {
	d := deploy(t, context.Background())
	go d.env.Serve(d)
	testComponents(d)
}

func TestFailActivateComponent(t *testing.T) {
	ctx := context.Background()
	d := deploy(t, ctx)
	go d.env.Serve(d)

	// Fail ActivateComponent a number of times.
	const n = 3
	failures := map[string]int{}
	d.activateComponent = func(ctx context.Context, req *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error) {
		if failures[req.Component] < n {
			failures[req.Component]++
			return nil, fmt.Errorf("simulated ActivateComponent(%q) failure", req.Component)
		}

		routing := &protos.UpdateRoutingInfoRequest{RoutingInfo: &protos.RoutingInfo{Component: req.Component, Local: true}}
		if _, err := d.wlet.UpdateRoutingInfo(routing); err != nil {
			return nil, err
		}
		components := &protos.UpdateComponentsRequest{Components: []string{req.Component}}
		if _, err := d.wlet.UpdateComponents(components); err != nil {
			return nil, err
		}
		return &protos.ActivateComponentReply{}, nil
	}

	testComponents(d)
}

func TestFailGetListenerAddress(t *testing.T) {
	t.Skip("TODO(mwhittaker): Make this test pass.")

	ctx := context.Background()
	d := deploy(t, ctx)
	go d.env.Serve(d)

	// Fail GetListenerAddress a number of times.
	const n = 3
	failures := map[string]int{}
	d.getListenerAddress = func(ctx context.Context, req *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
		if failures[req.Name] < n {
			failures[req.Name]++
			return nil, fmt.Errorf("simulated GetListenerAddress(%q) failure", req.Name)
		}
		return &protos.GetListenerAddressReply{Address: ":0"}, nil
	}

	testComponents(d)
}

func TestGetListenerAddressReturnsInvalidAddress(t *testing.T) {
	t.Skip("TODO(mwhittaker): Make this test pass.")

	ctx := context.Background()
	d := deploy(t, ctx)
	go d.env.Serve(d)

	// Return an invalid listener a number of times.
	const n = 3
	failures := map[string]int{}
	d.getListenerAddress = func(ctx context.Context, req *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
		if failures[req.Name] < n {
			failures[req.Name]++
			return &protos.GetListenerAddressReply{Address: "this is not a valid address"}, nil
		}
		return &protos.GetListenerAddressReply{Address: ":0"}, nil
	}

	testComponents(d)
}

func TestGetListenerAddressReturnsAddressAlreadyInUse(t *testing.T) {
	t.Skip("TODO(mwhittaker): Make this test pass.")

	// Listen on port 45678.
	lis, err := net.Listen("tcp", "localhost:45678")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	ctx := context.Background()
	d := deploy(t, ctx)
	go d.env.Serve(d)

	// Tell the weavelet to listen on port 45678 a number of times.
	const n = 3
	failures := map[string]int{}
	d.getListenerAddress = func(ctx context.Context, req *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
		if failures[req.Name] < n {
			failures[req.Name]++
			return &protos.GetListenerAddressReply{Address: "localhost:45678"}, nil
		}
		return &protos.GetListenerAddressReply{Address: ":0"}, nil
	}
	testComponents(d)
}

func TestFailExportListener(t *testing.T) {
	ctx := context.Background()
	d := deploy(t, ctx)
	go d.env.Serve(d)

	// Fail ExportListener a number of times.
	const n = 3
	failures := map[string]int{}
	d.exportListener = func(ctx context.Context, req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
		if failures[req.Listener] < n {
			failures[req.Listener]++
			return nil, fmt.Errorf("simulated ExportListener(%q) error", req.Listener)
		}
		return &protos.ExportListenerReply{}, nil
	}

	testComponents(d)
}

func TestExportListenerReturnsError(t *testing.T) {
	t.Skip("TODO(mwhittaker): Make this test pass.")

	ctx := context.Background()
	d := deploy(t, ctx)
	go d.env.Serve(d)

	// Return an error from ExportListener a number of times.
	const n = 3
	failures := map[string]int{}
	d.exportListener = func(ctx context.Context, req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
		if failures[req.Listener] < n {
			failures[req.Listener]++
			return &protos.ExportListenerReply{Error: fmt.Sprintf("simulated ExportListener(%q) error", req.Listener)}, nil
		}
		return &protos.ExportListenerReply{}, nil
	}

	testComponents(d)
}

func TestUpdateMissingComponents(t *testing.T) {
	ctx := context.Background()
	d := deploy(t, ctx)
	go d.env.Serve(d)

	// Update the weavelet with components that don't exist.
	components := &protos.UpdateComponentsRequest{Components: []string{"foo", "bar"}}
	if _, err := d.wlet.UpdateComponents(components); err != nil {
		// TODO(mwhittaker): Right now, UpdateComponents always returns nil and
		// updates components in the background. This is to avoid inducing
		// deadlock in deployers. We should probably return an error here when
		// possible.
		t.Fatal(err)
	}

	testComponents(d)
}

func TestUpdateExistingComponents(t *testing.T) {
	ctx := context.Background()
	d := deploy(t, ctx)
	go d.env.Serve(d)
	testComponents(d)

	// Update the weavelet with components that have already been started.
	components := &protos.UpdateComponentsRequest{
		Components: []string{
			"github.com/ServiceWeaver/weaver/internal/testdeployer/a",
			"github.com/ServiceWeaver/weaver/internal/testdeployer/b",
			"github.com/ServiceWeaver/weaver/internal/testdeployer/c",
		},
	}
	if _, err := d.wlet.UpdateComponents(components); err != nil {
		t.Fatal(err)
	}

	testComponents(d)
}

func TestUpdateNilRoutingInfo(t *testing.T) {
	t.Skip("TODO(mwhittaker): Make this test pass.")

	ctx := context.Background()
	d := deploy(t, ctx)
	go d.env.Serve(d)

	// Update the weavelet with a nil routing info.
	routing := &protos.UpdateRoutingInfoRequest{}
	if _, err := d.wlet.UpdateRoutingInfo(routing); err == nil {
		t.Fatal("UpdateRoutingInfo: unexpected success")
	}

	testComponents(d)
}

func TestUpdateRoutingInfoMissingComponent(t *testing.T) {
	ctx := context.Background()
	d := deploy(t, ctx)
	go d.env.Serve(d)

	// Update the weavelet with routing info for a component that doesn't
	// exist.
	routing := &protos.UpdateRoutingInfoRequest{
		RoutingInfo: &protos.RoutingInfo{
			Component: "foo",
			Local:     true,
		},
	}
	if _, err := d.wlet.UpdateRoutingInfo(routing); err == nil {
		t.Fatal("UpdateRoutingInfo: unexpected success")
	}

	testComponents(d)
}

func TestUpdateRoutingInfoNotStartedComponent(t *testing.T) {
	ctx := context.Background()
	d := deploy(t, ctx)
	go d.env.Serve(d)

	// Update the weavelet with routing info for a component that has hasn't
	// started yet.
	routing := &protos.UpdateRoutingInfoRequest{
		RoutingInfo: &protos.RoutingInfo{
			Component: "github.com/ServiceWeaver/weaver/internal/testdeployer/a",
			Local:     true,
		},
	}
	if _, err := d.wlet.UpdateRoutingInfo(routing); err != nil {
		t.Fatal(err)
	}
	testComponents(d)
}

func TestUpdateLocalRoutingInfoWithNonLocal(t *testing.T) {
	t.Skip("TODO(mwhittaker): Make this test pass.")

	ctx := context.Background()
	d := deploy(t, ctx)
	go d.env.Serve(d)
	testComponents(d)

	// Update the weavelet with non-local routing info for a component, even
	// though the component has already started with local routing info. Today,
	// that is not allowed and should fail.
	routing := &protos.UpdateRoutingInfoRequest{
		RoutingInfo: &protos.RoutingInfo{
			Component: "github.com/ServiceWeaver/weaver/internal/testdeployer/a",
		},
	}
	if _, err := d.wlet.UpdateRoutingInfo(routing); err == nil {
		t.Fatal("UpdateRoutingInfo: unexpected success")
	}
	testComponents(d)
}
