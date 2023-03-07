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

package impl

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protos"

	"github.com/ServiceWeaver/weaver/internal/logtype"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/ServiceWeaver/weaver/internal/proto"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime/envelope"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/retry"
)

// babysitter starts and manages weavelets belonging to a single colocation
// group for a single application version, on the local machine.
type babysitter struct {
	ctx           context.Context
	opts          envelope.Options
	dep           *protos.Deployment
	group         *protos.ColocationGroup
	replicaId     int32
	mgrAddr       string
	logger        logtype.Logger
	traceExporter *traceio.Writer // to export traces to the manager

	mu sync.RWMutex
	// Envelopes managed by the babysitter, keyed by the Service Weaver process they
	// correspond to.
	envelopes map[string]*envelope.Envelope
}

var _ envelope.EnvelopeHandler = &babysitter{}

// RunBabysitter creates and runs a babysitter that was deployed using SSH.
func RunBabysitter(ctx context.Context) error {
	// Retrieve the deployment information.
	depInfo := &BabysitterInfo{}
	if err := proto.FromEnv(os.Getenv(BabysitterInfoKey), depInfo); err != nil {
		return fmt.Errorf("unable to retrieve deployment info: %w", err)
	}

	// Create the log saver.
	fs, err := logging.NewFileStore(depInfo.LogDir)
	if err != nil {
		return fmt.Errorf("cannot create log storage: %w", err)
	}
	logSaver := fs.Add

	bs := &babysitter{
		ctx:       ctx,
		dep:       depInfo.Deployment,
		group:     depInfo.Group,
		replicaId: depInfo.ReplicaId,
		mgrAddr:   depInfo.ManagerAddr,
		logger: logging.FuncLogger{
			Opts: logging.Options{
				App:        depInfo.Deployment.App.Name,
				Deployment: depInfo.Deployment.Id,
				Component:  "Babysitter",
				Weavelet:   uuid.NewString(),
				Attrs:      []string{"serviceweaver/system", ""},
			},
			Write: logSaver,
		},
		traceExporter: traceio.NewWriter(func(spans *protos.Spans) error {
			return protomsg.Call(ctx, protomsg.CallArgs{
				Client:  http.DefaultClient,
				Addr:    depInfo.ManagerAddr,
				URLPath: recvTraceSpansURL,
				Request: spans,
			})
		}),
		opts:      envelope.Options{Restart: envelope.OnFailure, Retry: retry.DefaultOptions},
		envelopes: map[string]*envelope.Envelope{},
	}
	go bs.collectMetrics()
	bs.run()
	return nil
}

func (b *babysitter) collectMetrics() {
	tickerCollectMetrics := time.NewTicker(time.Minute)
	defer tickerCollectMetrics.Stop()
	for {
		select {
		case <-tickerCollectMetrics.C:
			var ms []*metrics.MetricSnapshot
			for _, e := range b.envelopes {
				m, err := e.ReadMetrics()
				if err != nil {
					b.logger.Error("Unable to collect metrics", err, "weavelet", e.Weavelet().Id)
					continue
				}
				ms = append(ms, m...)
			}
			ms = append(ms, metrics.Snapshot()...)

			// Convert metrics to metrics protos.
			var metrics []*protos.MetricSnapshot
			for _, m := range ms {
				metrics = append(metrics, m.ToProto())
			}
			if err := protomsg.Call(b.ctx, protomsg.CallArgs{
				Client:  http.DefaultClient,
				Addr:    b.mgrAddr,
				URLPath: recvMetricsURL,
				Request: &BabysitterMetrics{GroupName: b.group.Name, ReplicaId: b.replicaId, Metrics: metrics},
			}); err != nil {
				b.logger.Error("Error collecting metrics", err)
			}
		case <-b.ctx.Done():
			return
		}
	}
}

// run runs the Envelope management loop. This call will block until the
// context passed to the babysitter is canceled.
func (b *babysitter) run() error {
	host, err := os.Hostname()
	if err != nil {
		return err
	}

	b.logger.Info("Running babysitter to manage", "group", b.group.Name, "location", host)
	var version string
	for r := retry.Begin(); r.Continue(b.ctx); {
		procsToStart, newVersion, err := b.getProcessesToStart(b.ctx, version)
		if err != nil {
			continue
		}
		version = newVersion
		for _, proc := range procsToStart {
			if err := b.startProcess(proc); err != nil {
				b.logger.Error("Error starting", err, "process", proc)
			}
		}
		r.Reset()
	}
	return b.ctx.Err()
}

func (b *babysitter) getProcessesToStart(ctx context.Context, version string) ([]string, string, error) {
	request := &protos.GetProcessesToStartRequest{
		App:             b.dep.App.Name,
		DeploymentId:    b.dep.Id,
		ColocationGroup: b.group.Name,
		Version:         version,
	}
	reply := &protos.GetProcessesToStartReply{}
	err := protomsg.Call(ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.mgrAddr,
		URLPath: getProcessesToStartURL,
		Request: request,
		Reply:   reply,
	})
	if err != nil {
		return nil, "", err
	}
	if reply.Unchanged {
		return nil, "", fmt.Errorf("no new processes to start")
	}
	return reply.Processes, reply.Version, nil
}

func (b *babysitter) startProcess(proc string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.envelopes[proc]; ok {
		// Already started.
		return nil
	}

	// Start the weavelet and capture its logs, traces, and metrics.
	id := uuid.New().String()
	wlet := &protos.WeaveletInfo{
		App:               b.dep.App.Name,
		DeploymentId:      b.dep.Id,
		Group:             b.group,
		GroupId:           id,
		Process:           proc,
		Id:                id,
		SameProcess:       b.dep.App.SameProcess,
		Sections:          b.dep.App.Sections,
		SingleProcess:     b.dep.SingleProcess,
		UseLocalhost:      b.dep.UseLocalhost,
		ProcessPicksPorts: b.dep.ProcessPicksPorts,
		NetworkStorageDir: b.dep.NetworkStorageDir,
	}

	e, err := envelope.NewEnvelope(wlet, b.dep.App, b, b.opts)
	if err != nil {
		return err
	}
	go func() {
		if err := e.Run(b.ctx); err != nil {
			b.logger.Error("e.Run", err)
		}
	}()
	b.envelopes[proc] = e
	return nil
}

// StartComponent implements the protos.EnvelopeHandler interface.
func (b *babysitter) StartComponent(req *protos.ComponentToStart) error {
	return protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.mgrAddr,
		URLPath: startComponentURL,
		Request: req,
	})
}

// StartColocationGroup implements the protos.EnvelopeHandler interface.
func (b *babysitter) StartColocationGroup(req *protos.ColocationGroup) error {
	return protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.mgrAddr,
		URLPath: startColocationGroupManagerURL,
		Request: req,
	})
}

// RegisterReplica implements the protos.EnvelopeHandler interface.
func (b *babysitter) RegisterReplica(replica *protos.ReplicaToRegister) error {
	b.logger.Info("Process (re)started with new address",
		"process", logging.ShortenComponent(replica.Process),
		"address", replica.Address)
	return protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.mgrAddr,
		URLPath: registerReplicaURL,
		Request: replica,
	})
}

// ReportLoad implements the protos.EnvelopeHandler interface.
func (b *babysitter) ReportLoad(*protos.WeaveletLoadReport) error {
	return nil
}

// ExportListener implements the protos.EnvelopeHandler interface.
func (b *babysitter) ExportListener(req *protos.ListenerToExport) (*protos.ExportListenerReply, error) {
	reply := &protos.ExportListenerReply{}
	if err := protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.mgrAddr,
		URLPath: exportListenerURL,
		Request: req,
		Reply:   reply,
	}); err != nil {
		return nil, err
	}
	return reply, nil
}

// GetRoutingInfo implements the protos.EnvelopeHandler interface.
func (b *babysitter) GetRoutingInfo(req *protos.GetRoutingInfo) (*protos.RoutingInfo, error) {
	reply := &protos.RoutingInfo{}
	if err := protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.mgrAddr,
		URLPath: getRoutingInfoURL,
		Request: req,
		Reply:   reply,
	}); err != nil {
		return nil, err
	}
	return reply, nil
}

// GetComponentsToStart implements the protos.EnvelopeHandler interface.
func (b *babysitter) GetComponentsToStart(req *protos.GetComponentsToStart) (*protos.ComponentsToStart, error) {
	reply := &protos.ComponentsToStart{}
	err := protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.mgrAddr,
		URLPath: getComponentsToStartURL,
		Request: req,
		Reply:   reply,
	})
	return reply, err
}

// RecvLogEntry implements the protos.EnvelopeHandler interface.
func (b *babysitter) RecvLogEntry(req *protos.LogEntry) {
	protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.mgrAddr,
		URLPath: recvLogEntryURL,
		Request: req,
	})
}

// RecvTraceSpans implements the protos.EnvelopeHandler interface.
func (b *babysitter) RecvTraceSpans(spans []trace.ReadOnlySpan) error {
	return b.traceExporter.ExportSpans(b.ctx, spans)
}
