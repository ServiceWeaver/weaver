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

// Package envelope implements a sidecar-like process that connects a weavelet
// to its environment.
package envelope

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/control"
	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/deployers"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/version"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	// We rely on the weaver.controller component registrattion entry.
	_ "github.com/ServiceWeaver/weaver"
)

// EnvelopeHandler handles messages from the weavelet. Values passed to the
// handlers are only valid for the duration of the handler's execution.
type EnvelopeHandler interface {
	// ActivateComponent ensures that the provided component is running
	// somewhere. A call to ActivateComponent also implicitly signals that a
	// weavelet is interested in receiving routing info for the component.
	ActivateComponent(context.Context, *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error)

	// GetListenerAddress returns the address the weavelet should listen on for
	// a particular listener.
	GetListenerAddress(context.Context, *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error)

	// ExportListener exports the provided listener. Exporting a listener
	// typically, but not always, involves running a proxy that forwards
	// traffic to the provided address.
	ExportListener(context.Context, *protos.ExportListenerRequest) (*protos.ExportListenerReply, error)

	// GetSelfCertificate returns the certificate and the private key the
	// weavelet should use for network connection establishment. The weavelet
	// will issue this request each time it establishes a connection with
	// another weavelet.
	// NOTE: This method is only called if mTLS was enabled for the weavelet,
	// by passing it a WeaveletArgs with mtls=true.
	GetSelfCertificate(context.Context, *protos.GetSelfCertificateRequest) (*protos.GetSelfCertificateReply, error)

	// VerifyClientCertificate verifies the certificate chain presented by
	// a network client attempting to connect to the weavelet. It returns an
	// error if the network connection should not be established with the
	// client. Otherwise, it returns the list of weavelet components that the
	// client is authorized to invoke methods on.
	//
	// NOTE: This method is only called if mTLS was enabled for the weavelet,
	// by passing it a WeaveletArgs with mtls=true.
	VerifyClientCertificate(context.Context, *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error)

	// VerifyServerCertificate verifies the certificate chain presented by
	// the server the weavelet is attempting to connect to. It returns an
	// error iff the server identity doesn't match the identity of the specified
	// component.
	//
	// NOTE: This method is only called if mTLS was enabled for the weavelet,
	// by passing it a WeaveletArgs with mtls=true.
	VerifyServerCertificate(context.Context, *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error)

	// LogBatches handles a batch of log entries.
	LogBatch(context.Context, *protos.LogEntryBatch) error

	// HandleTraceSpans handles a set of trace spans.
	HandleTraceSpans(context.Context, *protos.TraceSpans) error
}

// Ensure that EnvelopeHandler implements all the DeployerControl methods.
var _ control.DeployerControl = EnvelopeHandler(nil)

// Envelope starts and manages a weavelet in a subprocess.
//
// For more information, refer to runtime/protos/runtime.proto and
// https://serviceweaver.dev/blog/deployers.html.
type Envelope struct {
	// Fields below are constant after construction.
	ctx          context.Context
	ctxCancel    context.CancelFunc
	logger       *slog.Logger
	tmpDir       string
	tmpDirOwned  bool // Did Envelope create tmpDir?
	myUds        string
	weavelet     *protos.WeaveletArgs
	weaveletAddr string
	config       *protos.AppConfig
	child        Child                   // weavelet process handle
	controller   control.WeaveletControl // Stub that talks to the weavelet controller

	// State needed to process metric updates.
	metricsMu sync.Mutex
	metrics   metrics.Importer
}

// Options contains optional arguments for the envelope.
type Options struct {
	// Override for temporary directory.
	TmpDir string

	// Logger is used for logging internal messages. If nil, a default logger is used.
	Logger *slog.Logger

	// Tracer is used for tracing internal calls. If nil, internal calls are not traced.
	Tracer trace.Tracer

	// Child is used to run the weavelet. If nil, a sub-process is created.
	Child Child
}

// NewEnvelope creates a new envelope, starting a weavelet subprocess (via child.Start) and
// establishing a bidirectional connection with it. The weavelet process can be
// stopped at any time by canceling the passed-in context.
//
// You can issue RPCs *to* the weavelet using the returned Envelope. To start
// receiving messages *from* the weavelet, call [Serve].
func NewEnvelope(ctx context.Context, wlet *protos.WeaveletArgs, config *protos.AppConfig, options Options) (*Envelope, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() { cancel() }() // cancel may be changed below if we want to delay it

	if options.Logger == nil {
		options.Logger = slog.Default()
	}

	// Make a temporary directory for unix domain sockets.
	var removeDir bool
	tmpDir := options.TmpDir
	tmpDirOwned := false
	if options.TmpDir == "" {
		var err error
		tmpDir, err = runtime.NewTempDir()
		if err != nil {
			return nil, err
		}
		tmpDirOwned = true
		runtime.OnExitSignal(func() { os.RemoveAll(tmpDir) }) // Cleanup when process exits

		// Arrange to delete tmpDir if this function returns an error.
		removeDir = true // Cleared on a successful return
		defer func() {
			if removeDir {
				os.RemoveAll(tmpDir)
			}
		}()
	}

	myUds := deployers.NewUnixSocketPath(tmpDir)

	wlet = protomsg.Clone(wlet)
	wlet.ControlSocket = deployers.NewUnixSocketPath(tmpDir)
	wlet.Redirects = []*protos.WeaveletArgs_Redirect{
		// Point weavelet at my control.DeployerControl component
		{
			Component: control.DeployerPath,
			Target:    control.DeployerPath,
			Address:   "unix://" + myUds,
		},
	}
	controller, err := getWeaveletControlStub(ctx, wlet.ControlSocket, options)
	if err != nil {
		return nil, err
	}
	e := &Envelope{
		ctx:         ctx,
		ctxCancel:   cancel,
		logger:      options.Logger,
		tmpDir:      tmpDir,
		tmpDirOwned: tmpDirOwned,
		myUds:       myUds,
		weavelet:    wlet,
		config:      config,
		controller:  controller,
	}

	child := options.Child
	if child == nil {
		child = &ProcessChild{}
	}
	if err := child.Start(ctx, e.config, e.weavelet); err != nil {
		return nil, fmt.Errorf("NewEnvelope: %w", err)
	}

	reply, err := controller.InitWeavelet(e.ctx, &protos.InitWeaveletRequest{
		Sections: config.Sections,
	})
	if err != nil {
		return nil, err
	}
	if err := verifyWeaveletInfo(reply); err != nil {
		return nil, err
	}
	e.weaveletAddr = reply.DialAddr

	e.child = child

	removeDir = false  // Serve() is now responsible for deletion
	cancel = func() {} // Delay real context cancellation
	return e, nil
}

// WeaveletControl returns the controller component for the weavelet managed by this envelope.
func (e *Envelope) WeaveletControl() control.WeaveletControl { return e.controller }

// Serve accepts incoming messages from the weavelet. RPC requests are handled
// serially in the order they are received. Serve blocks until the connection
// terminates, returning the error that caused it to terminate. You can cancel
// the connection by cancelling the context passed to [NewEnvelope]. This
// method never returns a non-nil error.
func (e *Envelope) Serve(h EnvelopeHandler) error {
	// Cleanup when we are done with the envelope.
	if e.tmpDirOwned {
		defer os.RemoveAll(e.tmpDir)
	}

	uds, err := net.Listen("unix", e.myUds)
	if err != nil {
		return err
	}

	var running errgroup.Group

	var stopErr error
	var once sync.Once
	stop := func(err error) {
		once.Do(func() {
			stopErr = err
		})
		e.ctxCancel()
	}

	// Capture stdout and stderr from the weavelet.
	if stdout := e.child.Stdout(); stdout != nil {
		running.Go(func() error {
			err := e.logLines("stdout", stdout, h)
			stop(err)
			return err
		})
	}
	if stderr := e.child.Stderr(); stderr != nil {
		running.Go(func() error {
			err := e.logLines("stderr", stderr, h)
			stop(err)
			return err
		})
	}

	// Start the goroutine watching the context for cancelation.
	running.Go(func() error {
		<-e.ctx.Done()
		err := e.ctx.Err()
		stop(err)
		return err
	})

	// Start the goroutine to handle deployer control calls.
	running.Go(func() error {
		err := deployers.ServeComponents(e.ctx, uds, e.logger, map[string]any{
			control.DeployerPath: h,
		})
		stop(err)
		return err
	})

	running.Wait()

	// Wait for the weavelet command to finish. This needs to be done after
	// we're done reading from stdout/stderr pipes, per comments on
	// exec.Cmd.StdoutPipe and exec.Cmd.StderrPipe.
	stop(e.child.Wait())

	return stopErr
}

// Pid returns the process id of the weavelet, if it is running in a separate process.
func (e *Envelope) Pid() (int, bool) {
	return e.child.Pid()
}

// WeaveletAddress returns the address that other components should dial to communicate with the
// weavelet.
func (e *Envelope) WeaveletAddress() string {
	return e.weaveletAddr
}

// GetHealth returns the health status of the weavelet.
func (e *Envelope) GetHealth() protos.HealthStatus {
	reply, err := e.controller.GetHealth(context.TODO(), &protos.GetHealthRequest{})
	if err != nil {
		return protos.HealthStatus_UNKNOWN
	}
	return reply.Status
}

// GetProfile gets a profile from the weavelet.
func (e *Envelope) GetProfile(req *protos.GetProfileRequest) ([]byte, error) {
	reply, err := e.controller.GetProfile(context.TODO(), req)
	if err != nil {
		return nil, err
	}
	return reply.Data, nil
}

// GetMetrics returns a weavelet's metrics.
func (e *Envelope) GetMetrics() ([]*metrics.MetricSnapshot, error) {
	req := &protos.GetMetricsRequest{}
	reply, err := e.controller.GetMetrics(context.TODO(), req)
	if err != nil {
		return nil, err
	}

	e.metricsMu.Lock()
	defer e.metricsMu.Unlock()
	return e.metrics.Import(reply.Update)
}

// GetLoad gets a load report from the weavelet.
func (e *Envelope) GetLoad() (*protos.LoadReport, error) {
	req := &protos.GetLoadRequest{}
	reply, err := e.controller.GetLoad(context.TODO(), req)
	if err != nil {
		return nil, err
	}
	return reply.Load, nil
}

// UpdateComponents updates the weavelet with the latest set of components it
// should be running.
func (e *Envelope) UpdateComponents(components []string) error {
	req := &protos.UpdateComponentsRequest{
		Components: components,
	}
	_, err := e.controller.UpdateComponents(context.TODO(), req)
	return err
}

// UpdateRoutingInfo updates the weavelet with a component's most recent
// routing info.
func (e *Envelope) UpdateRoutingInfo(routing *protos.RoutingInfo) error {
	req := &protos.UpdateRoutingInfoRequest{
		RoutingInfo: routing,
	}
	_, err := e.controller.UpdateRoutingInfo(context.TODO(), req)
	return err
}

func (e *Envelope) logLines(component string, src io.Reader, h EnvelopeHandler) error {
	// Fill partial log entry.
	entry := &protos.LogEntry{
		App:       e.weavelet.App,
		Version:   e.weavelet.DeploymentId,
		Component: component,
		Node:      e.weavelet.Id,
		Level:     component, // Either "stdout" or "stderr"
		File:      "",
		Line:      -1,
	}
	batch := &protos.LogEntryBatch{}
	batch.Entries = append(batch.Entries, entry)

	rdr := bufio.NewReader(src)
	for {
		line, err := rdr.ReadBytes('\n')
		// Note: both line and err may be present.
		if len(line) > 0 {
			entry.Msg = string(dropNewline(line))
			entry.TimeMicros = 0 // In case previous LogBatch mutated it
			if err := h.LogBatch(e.ctx, batch); err != nil {
				return err
			}
		}
		if err != nil {
			return fmt.Errorf("capture %s: %w", component, err)
		}
	}
}

func dropNewline(line []byte) []byte {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	return line
}

// getWeaveletControlStub returns a control.WeaveletControl that forwards calls to the controller
// component in the weavelet at the specified socket.
func getWeaveletControlStub(ctx context.Context, socket string, options Options) (control.WeaveletControl, error) {
	controllerReg, ok := codegen.Find(control.WeaveletPath)
	if !ok {
		return nil, fmt.Errorf("controller component (%s) not found", control.WeaveletPath)
	}
	controlEndpoint := call.Unix(socket)
	resolver := call.NewConstantResolver(controlEndpoint)
	opts := call.ClientOptions{Logger: options.Logger}
	conn, err := call.Connect(ctx, resolver, opts)
	if err != nil {
		return nil, err
	}
	// We skip waitUntilReady() and rely on automatic retries of methods
	stub := call.NewStub(control.WeaveletPath, controllerReg, conn, options.Tracer, 0)
	obj := controllerReg.ClientStubFn(stub, "envelope")
	return obj.(control.WeaveletControl), nil
}

// verifyWeaveletInfo verifies the information sent by the weavelet.
func verifyWeaveletInfo(wlet *protos.InitWeaveletReply) error {
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
		return fmt.Errorf("version mismatch: deployer's deployer API version %s is incompatible with app' deployer API version %s", version.DeployerVersion, got)
	}
	return nil
}
