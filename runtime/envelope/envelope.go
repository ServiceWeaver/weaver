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
	"syscall"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/logtype"
	"github.com/ServiceWeaver/weaver/internal/pipe"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"go.opentelemetry.io/otel/sdk/trace"
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

	// GetRoutingInfo returns the latest routing information for the weavelet.
	//
	// This is a blocking method that can be processed out-of-order w.r.t.
	// the other methods.
	GetRoutingInfo(request *protos.GetRoutingInfo) (*protos.RoutingInfo, error)

	// GetComponentsToStart is a blocking call that returns the latest set of
	// components that should be started by the weavelet.
	//
	// This is a blocking method that can be processed out-of-order w.r.t.
	// the other methods.
	GetComponentsToStart(request *protos.GetComponentsToStart) (*protos.ComponentsToStart, error)

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
	conn     *conn.EnvelopeConn // conn to weavelet
	cmd      *pipe.Cmd          // command that started the weavelet
	weavelet *protos.WeaveletSetupInfo
	config   *protos.AppConfig
	handler  EnvelopeHandler
	logger   logtype.Logger

	mu        sync.Mutex // guards the following fields
	stopped   bool       // has Stop() been called?
	profiling bool       // are we currently collecting a profile?
}

// NewEnvelope creates a new envelope.
func NewEnvelope(ctx context.Context, wlet *protos.WeaveletSetupInfo, config *protos.AppConfig, h EnvelopeHandler) (*Envelope, error) {
	if h == nil {
		return nil, fmt.Errorf(
			"unable to create envelope for group %s due to nil handler",
			logging.ShortenComponent(wlet.Group.Name))
	}
	logger := logging.FuncLogger{
		Opts: logging.Options{
			App:        wlet.App,
			Deployment: wlet.DeploymentId,
			Component:  "envelope",
			Weavelet:   wlet.Id,
			Attrs:      []string{"serviceweaver/system", ""},
		},
		Write: h.RecvLogEntry,
	}
	e := &Envelope{
		weavelet: wlet,
		config:   config,
		handler:  h,
		logger:   logger,
	}
	if err := e.init(ctx); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Envelope) init(ctx context.Context) error {
	// Form the command.
	cmd := pipe.CommandContext(ctx, e.config.Binary, e.config.Args...)

	// Create the pipes first, so we can fill env and detect any errors early.
	// Pipe for messages to weavelet.
	toWeaveletFd, toWeavelet, err := cmd.WPipe()
	if err != nil {
		return fmt.Errorf("cannot create weavelet request pipe: %w", err)
	}
	// Pipe for messages to envelope.
	toEnvelopeFd, toEnvelope, err := cmd.RPipe()
	if err != nil {
		return fmt.Errorf("cannot create weavelet response pipe: %w", err)
	}

	// Create pipes that capture child outputs.
	outpipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	errpipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", runtime.ToWeaveletKey, strconv.FormatUint(uint64(toWeaveletFd), 10)))
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", runtime.ToEnvelopeKey, strconv.FormatUint(uint64(toEnvelopeFd), 10)))
	cmd.Env = append(cmd.Env, e.config.Env...)

	// Start the command.
	if err := cmd.Start(); err != nil {
		return err
	}

	// Capture stdout and stderr from the weavelet.
	// TODO(spetrovic): These need to be terminated and their errors taken into
	// account. Fix along with fixing the Stop() behavior.
	go func() {
		if err := e.copyLines("stdout", outpipe); err != nil {
			fmt.Fprintf(os.Stderr, "Error copying stdout: %v\n", err)
		}
	}()
	go func() {
		if err := e.copyLines("stderr", errpipe); err != nil {
			fmt.Fprintf(os.Stderr, "Error copying stdout: %v\n", err)
		}
	}()

	// Create the connection, now that the weavelet has (hopefully) started.
	conn, err := conn.NewEnvelopeConn(toEnvelope, toWeavelet, e.handler, e.weavelet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to start envelope conn: %v\n", err)
		return err
	}

	e.cmd = cmd
	e.conn = conn
	return nil
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

// Serve blocks accepting incoming messages from the weavelet.
func (e *Envelope) Serve(ctx context.Context) error {
	defer e.cmd.Cleanup()
	connErr := e.conn.Serve() // blocks
	cmdErr := e.cmd.Wait()    // blocks
	for _, err := range []error{connErr, cmdErr} {
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) && !errors.Is(err, syscall.ECHILD) {
			return err
		}
	}
	return nil
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
	prof, err := e.conn.DoProfilingRPC(req)
	if err != nil {
		return nil, err
	}
	if len(prof.Data) == 0 && len(prof.Errors) > 0 {
		return nil, fmt.Errorf("profiled with errors: %v", prof.Errors)
	}
	return prof, nil
}

// Stop permanently terminates the weavelet process managed by the envelope.
func (e *Envelope) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopped = true
	if err := e.cmd.Process.Kill(); err != nil {
		e.logger.Error("Failed to kill process", err, "pid", e.cmd.Process.Pid)
		return err
	}
	// NOTE(mwhittaker): Serve also calls Wait. The redundant wait leads
	// to a "waitid: no child processes" error which we ignore.
	if _, err := e.cmd.Process.Wait(); err != nil && !errors.Is(err, syscall.ECHILD) {
		e.logger.Error("Failed to kill process", err, "pid", e.cmd.Process.Pid)
		return err
	}
	e.logger.Debug("Killed process", "pid", e.cmd.Process.Pid)
	return nil
}

// ReadMetrics returns the set of all captured metrics.
func (e *Envelope) ReadMetrics() ([]*metrics.MetricSnapshot, error) {
	return e.conn.GetMetricsRPC()
}

// GetLoadInfo returns the latest load information at the weavelet.
func (e *Envelope) GetLoadInfo() (*protos.WeaveletLoadReport, error) {
	return e.conn.GetLoadInfoRPC()
}

func (e *Envelope) copyLines(component string, src io.Reader) error {
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
			e.handler.RecvLogEntry(entry)
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
