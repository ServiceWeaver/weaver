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
	"os"
	"strconv"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/pipe"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/sync/errgroup"
)

// EnvelopeHandler implements the envelope side processing of messages
// exchanged with the managed weavelet.
type EnvelopeHandler interface {
	// StartComponent starts the given component.
	StartComponent(entry *protos.ComponentToStart) error

	// GetAddress gets the address a weavelet should listen on for a listener.
	GetAddress(req *protos.GetAddressRequest) (*protos.GetAddressReply, error)

	// ExportListener exports the given listener.
	ExportListener(req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error)

	// RecvLogEntry enables the envelope to receive a log entry.
	RecvLogEntry(entry *protos.LogEntry)

	// RecvTraceSpans enables the envelope to receive a sequence of trace spans.
	RecvTraceSpans(spans []trace.ReadOnlySpan) error
}

// Ensure that EnvelopeHandler remains in-sync with conn.EnvelopeHandler.
var (
	_ EnvelopeHandler      = conn.EnvelopeHandler(nil)
	_ conn.EnvelopeHandler = EnvelopeHandler(nil)
)

// Envelope starts and manages a weavelet, i.e., an OS process running inside a
// colocation group replica, hosting Service Weaver components. It also captures the
// weavelet's tracing, logging, and metrics information.
type Envelope struct {
	// Fields below are constant after construction.
	ctx        context.Context
	ctxCancel  context.CancelFunc
	weavelet   *protos.WeaveletSetupInfo
	config     *protos.AppConfig
	conn       *conn.EnvelopeConn // conn to weavelet
	cmd        *pipe.Cmd          // command that started the weavelet
	stdoutPipe io.ReadCloser      // stdout pipe from the weavelet
	stderrPipe io.ReadCloser      // stderr pipe from the weavelet

	mu        sync.Mutex // guards the following fields
	profiling bool       // are we currently collecting a profile?
}

// NewEnvelope creates a new envelope, starting a weavelet process and
// establishing a bidirectional connection with it. The weavelet process can be
// stopped at any time by canceling the passed-in context.
func NewEnvelope(ctx context.Context, wlet *protos.WeaveletSetupInfo, config *protos.AppConfig) (*Envelope, error) {
	ctx, cancel := context.WithCancel(ctx)
	e := &Envelope{
		ctx:       ctx,
		ctxCancel: cancel,
		weavelet:  wlet,
		config:    config,
	}

	// Form the weavelet command.
	cmd := pipe.CommandContext(e.ctx, e.config.Binary, e.config.Args...)

	// Create the pipes first, so we can fill cmd.Env and detect any errors early.
	//
	// Pipe for messages to weavelet.
	toWeaveletFd, toWeavelet, err := cmd.WPipe()
	if err != nil {
		return nil, fmt.Errorf("cannot create weavelet request pipe: %w", err)
	}
	// Pipe for messages to envelope.
	toEnvelopeFd, toEnvelope, err := cmd.RPipe()
	if err != nil {
		return nil, fmt.Errorf("cannot create weavelet response pipe: %w", err)
	}

	// Create pipes that capture child outputs.
	outpipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	errpipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", runtime.ToWeaveletKey, strconv.FormatUint(uint64(toWeaveletFd), 10)))
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", runtime.ToEnvelopeKey, strconv.FormatUint(uint64(toEnvelopeFd), 10)))
	cmd.Env = append(cmd.Env, e.config.Env...)

	// Start the command.
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Create the connection, now that the weavelet is running.
	conn, err := conn.NewEnvelopeConn(e.ctx, toEnvelope, toWeavelet, e.weavelet)
	if err != nil {
		return nil, err
	}

	e.cmd = cmd
	e.conn = conn
	e.stdoutPipe = outpipe
	e.stderrPipe = errpipe
	return e, nil
}

// Serve accepts incoming messages from the weavelet. Messages that are received
// are handled as an ordered sequence. This call blocks until the envelope
// terminates, returning the error that caused it to terminate. This method will
// never return a non-nil error.
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

	running.Wait() //nolint:errcheck // supplanted by stopErr

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

// WeaveletInfo returns information about the started weavelet.
func (e *Envelope) WeaveletInfo() *protos.WeaveletInfo {
	return e.conn.WeaveletInfo()
}

// HealthStatus returns the health status of the weavelet.
func (e *Envelope) HealthStatus() protos.HealthStatus {
	healthStatus, err := e.conn.HealthStatusRPC()
	if err != nil {
		return protos.HealthStatus_UNHEALTHY
	}
	return healthStatus
}

// RunProfiling returns weavelet profiling information.
func (e *Envelope) RunProfiling(_ context.Context, req *protos.RunProfiling) (*protos.Profile, error) {
	if ok := e.toggleProfiling(false); !ok {
		return nil, fmt.Errorf("profiling already in progress")
	}
	defer e.toggleProfiling(true)
	return e.conn.DoProfilingRPC(req)
}

// ReadMetrics returns the set of all captured metrics.
func (e *Envelope) ReadMetrics() ([]*metrics.MetricSnapshot, error) {
	return e.conn.GetMetricsRPC()
}

// GetLoadInfo returns the latest load information at the weavelet.
func (e *Envelope) GetLoadInfo() (*protos.WeaveletLoadReport, error) {
	return e.conn.GetLoadInfoRPC()
}

// UpdateComponents updates the weavelet with the latest set of components it
// should be running.
func (e *Envelope) UpdateComponents(components []string) error {
	return e.conn.UpdateComponentsRPC(&protos.ComponentsToStart{Components: components})
}

// UpdateRoutingInfo updates the weavelet with a component's most recent
// routing info.
func (e *Envelope) UpdateRoutingInfo(info *protos.RoutingInfo) error {
	return e.conn.UpdateRoutingInfoRPC(info)
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
			h.RecvLogEntry(entry)
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
