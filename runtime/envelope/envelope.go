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
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"go.opentelemetry.io/otel/sdk/trace"
)

// EnvelopeHandler implements the envelope side processing of messages
// exchanged with the managed weavelet.
type EnvelopeHandler interface {
	// StartComponent starts the given component.
	StartComponent(entry *protos.ComponentToStart) error

	// RegisterReplica registers the given weavelet replica.
	RegisterReplica(entry *protos.ReplicaToRegister) error

	// ReportLoad reports the given weavelet load information.
	ReportLoad(entry *protos.WeaveletLoadReport) error

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

// RestartPolicy governs when a weavelet is restarted.
type RestartPolicy int

const (
	Never     RestartPolicy = iota // Never restart
	Always                         // Always restart
	OnFailure                      // Restart iff weavelet exits with non-zero exit code
)

func (r RestartPolicy) String() string {
	switch r {
	case Never:
		return "Never"
	case Always:
		return "Always"
	case OnFailure:
		return "OnFailure"
	default:
		panic(fmt.Errorf("invalid restart policy %d", r))
	}
}

// Options to configure the weavelet managed by the Envelope.
type Options struct {
	// Restart dictates when a weavelet is restarted. Defaults to Never.
	Restart RestartPolicy

	// Retry configures the exponential backoff performed when restarting the
	// weavelet. Defaults to retry.DefaultOptions.
	Retry retry.Options

	// GetEnvelopeConn returns a connection between a pre-started weavelet and
	// its Envelope.
	//
	// Note that setting this option implies that the weavelet has already been
	// started, in which case the Envelope will not restart it (i.e., the
	// Restart policy is assumed to be Never).
	//
	// TODO: conn.EnvelopeConn is internal. This is only used right now for
	// weavertest. If we remove the weavertest use case, we can remove this
	// option.
	//
	// TODO: Set this only in weavertest.
	GetEnvelopeConn func() *conn.EnvelopeConn
}

// Envelope starts and manages a weavelet, i.e., an OS process running inside a
// colocation group replica, hosting Service Weaver components. It also captures the
// weavelet's tracing, logging, and metrics information.
type Envelope struct {
	// Fields below are constant after construction.
	weavelet *protos.WeaveletInfo
	config   *protos.AppConfig
	handler  EnvelopeHandler
	opts     Options
	logger   logtype.Logger

	mu        sync.Mutex         // guards the following fields
	process   *os.Process        // the currently running subprocess, or nil
	conn      *conn.EnvelopeConn // conn to weavelet
	stopped   bool               // has Stop() been called?
	profiling bool               // are we currently collecting a profile?
}

func NewEnvelope(wlet *protos.WeaveletInfo, config *protos.AppConfig, h EnvelopeHandler, opts Options) (*Envelope, error) {
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
	return &Envelope{
		weavelet: wlet,
		config:   config,
		handler:  h,
		opts:     opts,
		logger:   logger,
	}, nil
}

// getConn returns the current connection to the weavelet.
func (e *Envelope) getConn() *conn.EnvelopeConn {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.conn
}

// setConn sets the current connection to the weavelet.
func (e *Envelope) setConn(conn *conn.EnvelopeConn) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.conn = conn
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

// Run runs the application, restarting it when necessary.
func (e *Envelope) Run(ctx context.Context) error {
	if e.opts.GetEnvelopeConn != nil {
		// The weavelet already started. Set the connection between the
		// weavelet and the envelope.
		conn := e.opts.GetEnvelopeConn()
		e.setConn(conn)
		return ctx.Err()
	}

	for r := retry.BeginWithOptions(e.opts.Retry); r.Continue(ctx); {
		err := e.runWeavelet(ctx)
		if e.isStopped() {
			// When an envelope is stopped, it kills its subprocesses. These
			// subprocesses will report errors, expectedly, since they're
			// killed. We expect these errors, so we swallow them.
			return nil
		}
		if e.opts.Restart == Never || (e.opts.Restart == OnFailure && err == nil) {
			return err
		}
	}
	return ctx.Err()
}

// HealthStatus returns the health status of the weavelet.
func (e *Envelope) HealthStatus() protos.HealthStatus {
	conn := e.getConn()
	if conn == nil {
		return protos.HealthStatus_UNHEALTHY
	}
	healthStatus, err := conn.HealthStatusRPC()
	if err != nil {
		return protos.HealthStatus_UNHEALTHY
	}
	return healthStatus
}

// RunProfiling returns weavelet profiling information.
func (e *Envelope) RunProfiling(_ context.Context, req *protos.RunProfiling) (*protos.Profile, error) {
	conn := e.getConn()
	if conn == nil {
		return nil, fmt.Errorf("weavelet pipe is down")
	}
	if ok := e.toggleProfiling(false); !ok {
		return nil, fmt.Errorf("profiling already in progress")
	}
	defer e.toggleProfiling(true)
	prof, err := conn.DoProfilingRPC(req)
	if err != nil {
		return nil, err
	}
	if len(prof.Data) == 0 && len(prof.Errors) > 0 {
		return nil, fmt.Errorf("profiled with errors: %v", prof.Errors)
	}
	return prof, nil
}

// runWeavelet with the provided environment and wait for it to terminate.
func (e *Envelope) runWeavelet(ctx context.Context) error {
	// Form the command.
	cmd := pipe.CommandContext(ctx, e.config.Binary, e.config.Args...)
	defer cmd.Cleanup()

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

	conn, err := conn.NewEnvelopeConn(toEnvelope, toWeavelet, e.handler, e.weavelet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to start envelope conn: %v\n", err)
		return err
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
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", runtime.ToWeaveletKey, strconv.Itoa(toWeaveletFd)))
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", runtime.ToEnvelopeKey, strconv.Itoa(toEnvelopeFd)))
	cmd.Env = append(cmd.Env, e.config.Env...)

	// Different sources are read from by different go-routines.
	var stdoutErr, stderrErr, weaveletConnErr error
	var wait sync.WaitGroup
	wait.Add(2) // 2 for stdout and stderr
	go func() {
		defer wait.Done()
		stdoutErr = e.copyLines("stdout", outpipe)
	}()
	go func() {
		defer wait.Done()
		stderrErr = e.copyLines("stderr", errpipe)
	}()
	wait.Add(1)
	go func() {
		defer wait.Done()
		weaveletConnErr = conn.Run()
	}()

	// Start the command.
	if err := cmd.Start(); err != nil {
		return err
	}
	e.mu.Lock()
	e.process = cmd.Process
	e.mu.Unlock()

	// Set the connection only after the weavelet information was sent to the
	// subprocess. Otherwise, it is possible to send over the pipe information
	// (e.g., health checks) before the weavelet information was sent.
	e.setConn(conn)

	// Wait for the command to terminate.
	//
	// NOTE(mwhittaker): Stop also calls Wait. If Stop is called, then the
	// redundant wait leads to a "waitid: no child processes" error which we
	// ignore.
	wait.Wait()
	runErr := cmd.Wait()
	for _, err := range []error{runErr, stdoutErr, stderrErr, weaveletConnErr} {
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) && !errors.Is(err, syscall.ECHILD) {
			return err
		}
	}
	e.mu.Lock()
	e.process = nil
	e.conn = nil
	e.mu.Unlock()
	return nil
}

// Stop permanently terminates the weavelet process managed by the envelope.
func (e *Envelope) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopped = true
	if e.process == nil {
		return nil
	}
	if err := e.process.Kill(); err != nil {
		e.logger.Error("Failed to kill process", err, "pid", e.process.Pid)
		return err
	}
	// NOTE(mwhittaker): runWeavelet also calls Wait. The redundant wait leads
	// to a "waitid: no child processes" error which we ignore.
	if _, err := e.process.Wait(); err != nil && !errors.Is(err, syscall.ECHILD) {
		e.logger.Error("Failed to kill process", err, "pid", e.process.Pid)
		return err
	}
	e.logger.Debug("Killed process", "pid", e.process.Pid)
	return nil
}

// ReadMetrics returns the set of all captured metrics.
func (e *Envelope) ReadMetrics() ([]*metrics.MetricSnapshot, error) {
	conn := e.getConn()
	if conn == nil {
		return nil, fmt.Errorf("cannot read metrics: weavelet pipe is down")
	}
	return e.conn.GetMetricsRPC()
}

func (e *Envelope) isStopped() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopped
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

func (e *Envelope) Weavelet() *protos.WeaveletInfo {
	return e.weavelet
}

func dropNewline(line []byte) []byte {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	return line
}
