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
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/pipe"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"golang.org/x/sync/errgroup"
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
	// by passing it an EnvelopeInfo with mtls=true.
	GetSelfCertificate(context.Context, *protos.GetSelfCertificateRequest) (*protos.GetSelfCertificateReply, error)

	// VerifyClientCertificate verifies the certificate chain presented by
	// a network client attempting to connect to the weavelet. It returns an
	// error if the network connection should not be established with the
	// client. Otherwise, it returns the list of weavelet components that the
	// client is authorized to invoke methods on.
	//
	// NOTE: This method is only called if mTLS was enabled for the weavelet,
	// by passing it an EnvelopeInfo with mtls=true.
	VerifyClientCertificate(context.Context, *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error)

	// VerifyServerCertificate verifies the certificate chain presented by
	// the server the weavelet is attempting to connect to. It returns an
	// error iff the server identity doesn't match the identity of the specified
	// component.
	//
	// NOTE: This method is only called if mTLS was enabled for the weavelet,
	// by passing it an EnvelopeInfo with mtls=true.
	VerifyServerCertificate(context.Context, *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error)

	// HandleLogEntry handles a log entry.
	HandleLogEntry(context.Context, *protos.LogEntry) error

	// HandleTraceSpans handles a set of trace spans.
	HandleTraceSpans(context.Context, *protos.TraceSpans) error
}

// Ensure that EnvelopeHandler remains in-sync with conn.EnvelopeHandler.
var (
	_ EnvelopeHandler      = conn.EnvelopeHandler(nil)
	_ conn.EnvelopeHandler = EnvelopeHandler(nil)
)

// Envelope starts and manages a weavelet in a subprocess.
//
// For more information, refer to runtime/protos/runtime.proto and
// https://serviceweaver.dev/blog/deployers.html.
type Envelope struct {
	// Fields below are constant after construction.
	ctx        context.Context
	ctxCancel  context.CancelFunc
	weavelet   *protos.EnvelopeInfo
	config     *protos.AppConfig
	conn       *conn.EnvelopeConn // conn to weavelet
	cmd        *pipe.Cmd          // command that started the weavelet
	stdoutPipe io.ReadCloser      // stdout pipe from the weavelet
	stderrPipe io.ReadCloser      // stderr pipe from the weavelet

	mu        sync.Mutex // guards the following fields
	profiling bool       // are we currently collecting a profile?
}

// NewEnvelope creates a new envelope, starting a weavelet subprocess and
// establishing a bidirectional connection with it. The weavelet process can be
// stopped at any time by canceling the passed-in context.
//
// You can issue RPCs *to* the weavelet using the returned Envelope. To start
// receiving messages *from* the weavelet, call [Serve].
func NewEnvelope(ctx context.Context, wlet *protos.EnvelopeInfo, config *protos.AppConfig) (*Envelope, error) {
	ctx, cancel := context.WithCancel(ctx)
	e := &Envelope{
		ctx:       ctx,
		ctxCancel: cancel,
		weavelet:  wlet,
		config:    config,
	}

	// Form the weavelet command.
	cmd := pipe.CommandContext(e.ctx, e.config.Binary, e.config.Args...)

	// Create the request/response pipes first, so we can fill cmd.Env and detect any errors early.
	pipePair, err := cmd.MakePipePair()
	if err != nil {
		return nil, fmt.Errorf("NewEnvelope: create weavelet request/response pipes: %w", err)
	}

	// Create pipes that capture child outputs.
	outpipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("NewEnvelope: create stdout pipe: %w", err)
	}
	errpipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("NewEnvelope: create stderr pipe: %w", err)
	}

	// Create pair of pipes to use for component method calls from weavelet to envelope.

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", runtime.ToWeaveletKey, strconv.FormatUint(uint64(pipePair.ChildReader), 10)))
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", runtime.ToEnvelopeKey, strconv.FormatUint(uint64(pipePair.ChildWriter), 10)))
	cmd.Env = append(cmd.Env, e.config.Env...)

	// Start the command.
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("NewEnvelope: start subprocess: %w", err)
	}

	// Create the connection, now that the weavelet is running.
	conn, err := conn.NewEnvelopeConn(e.ctx, pipePair.ParentReader, pipePair.ParentWriter, e.weavelet)
	if err != nil {
		err := fmt.Errorf("NewEnvelope: connect to weavelet: %w", err)

		// Kill the subprocess, if it's not already dead.
		cancel()

		// Include stdout and stderr in the returned error.
		if bytes, stdoutErr := io.ReadAll(outpipe); stdoutErr == nil && len(bytes) > 0 {
			err = errors.Join(err, fmt.Errorf("-----BEGIN STDOUT-----\n%s-----END STDOUT-----", string(bytes)))
		}
		if bytes, stderrErr := io.ReadAll(errpipe); stderrErr == nil && len(bytes) > 0 {
			err = errors.Join(err, fmt.Errorf("-----BEGIN STDERR-----\n%s\n-----END STDERR-----", string(bytes)))
		}

		// Wait for the subprocess to terminate.
		if waitErr := cmd.Wait(); waitErr != nil {
			err = errors.Join(err, waitErr)
		}
		cmd.Cleanup()
		return nil, err
	}

	e.cmd = cmd
	e.conn = conn
	e.stdoutPipe = outpipe
	e.stderrPipe = errpipe
	return e, nil
}

// Serve accepts incoming messages from the weavelet. RPC requests are handled
// serially in the order they are received. Serve blocks until the connection
// terminates, returning the error that caused it to terminate. You can cancel
// the connection by cancelling the context passed to [NewEnvelope]. This
// method never returns a non-nil error.
func (e *Envelope) Serve(h EnvelopeHandler) error {
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
	running.Go(func() error {
		err := e.logLines("stdout", e.stdoutPipe, h)
		stop(err)
		return err
	})
	running.Go(func() error {
		err := e.logLines("stderr", e.stderrPipe, h)
		stop(err)
		return err
	})

	// Start the goroutine watching the context for cancelation.
	running.Go(func() error {
		<-e.ctx.Done()
		err := e.ctx.Err()
		stop(err)
		return err
	})

	// Start the goroutine to receive incoming messages.
	running.Go(func() error {
		err := e.conn.Serve(h)
		stop(err)
		return err
	})

	running.Wait()

	// Wait for the weavelet command to finish. This needs to be done after
	// we're done reading from stdout/stderr pipes, per comments on
	// exec.Cmd.StdoutPipe and exec.Cmd.StderrPipe.
	err := e.cmd.Wait()
	stop(err)
	e.cmd.Cleanup()

	return stopErr
}

// toggleProfiling compares the value of e.profiling to the given expected
// value, and if they are the same, toggles the value of e.profiling and
// returns true; otherwise, it leaves the value of e.profiling unchanged
// and returns false.
func (e *Envelope) toggleProfiling(expected bool) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.profiling != expected {
		return false
	}
	e.profiling = !e.profiling
	return true
}

// Pid returns the process id of the subprocess.
func (e *Envelope) Pid() int {
	return e.cmd.Process.Pid
}

// WeaveletInfo returns information about the started weavelet.
func (e *Envelope) WeaveletInfo() *protos.WeaveletInfo {
	return e.conn.WeaveletInfo()
}

// GetHealth returns the health status of the weavelet.
func (e *Envelope) GetHealth() protos.HealthStatus {
	status, err := e.conn.GetHealthRPC()
	if err != nil {
		return protos.HealthStatus_UNHEALTHY
	}
	return status
}

// GetProfile gets a profile from the weavelet.
func (e *Envelope) GetProfile(req *protos.GetProfileRequest) ([]byte, error) {
	if ok := e.toggleProfiling(false); !ok {
		return nil, fmt.Errorf("profiling already in progress")
	}
	defer e.toggleProfiling(true)
	return e.conn.GetProfileRPC(req)
}

// GetMetrics returns a weavelet's metrics.
func (e *Envelope) GetMetrics() ([]*metrics.MetricSnapshot, error) {
	return e.conn.GetMetricsRPC()
}

// GetLoad gets a load report from the weavelet.
func (e *Envelope) GetLoad() (*protos.LoadReport, error) {
	return e.conn.GetLoadRPC()
}

// UpdateComponents updates the weavelet with the latest set of components it
// should be running.
func (e *Envelope) UpdateComponents(components []string) error {
	return e.conn.UpdateComponentsRPC(components)
}

// UpdateRoutingInfo updates the weavelet with a component's most recent
// routing info.
func (e *Envelope) UpdateRoutingInfo(routing *protos.RoutingInfo) error {
	return e.conn.UpdateRoutingInfoRPC(routing)
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
	rdr := bufio.NewReader(src)
	for {
		line, err := rdr.ReadBytes('\n')
		// Note: both line and err may be present.
		if len(line) > 0 {
			entry.Msg = string(dropNewline(line))
			entry.TimeMicros = 0 // In case previous logSaver() call set it
			if err := h.HandleLogEntry(e.ctx, entry); err != nil {
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
