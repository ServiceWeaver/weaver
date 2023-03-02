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
	"time"

	"github.com/ServiceWeaver/weaver/internal/logtype"
	"github.com/ServiceWeaver/weaver/internal/proto"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime/envelope"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/sdk/trace"
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
	envelope      *envelope.Envelope
}

var _ envelope.EnvelopeHandler = &babysitter{}

// RunBabysitter creates and runs a babysitter that was deployed using SSH.
func RunBabysitter(ctx context.Context) error {
	// Retrieve the deployment information.
	info := &BabysitterInfo{}
	if err := proto.FromEnv(os.Getenv(babysitterInfoKey), info); err != nil {
		return fmt.Errorf("unable to retrieve deployment info: %w", err)
	}

	// Create the log saver.
	fs, err := logging.NewFileStore(info.LogDir)
	if err != nil {
		return fmt.Errorf("cannot create log storage: %w", err)
	}
	logSaver := fs.Add

	b := &babysitter{
		ctx:       ctx,
		dep:       info.Deployment,
		group:     info.Group,
		replicaId: info.ReplicaId,
		mgrAddr:   info.ManagerAddr,
		logger: logging.FuncLogger{
			Opts: logging.Options{
				App:        info.Deployment.App.Name,
				Deployment: info.Deployment.Id,
				Component:  "Babysitter",
				Weavelet:   uuid.NewString(),
				Attrs:      []string{"serviceweaver/system", ""},
			},
			Write: logSaver,
		},
		traceExporter: traceio.NewWriter(func(spans *protos.Spans) error {
			return protomsg.Call(ctx, protomsg.CallArgs{
				Client:  http.DefaultClient,
				Addr:    info.ManagerAddr,
				URLPath: recvTraceSpansURL,
				Request: spans,
			})
		}),
		opts: envelope.Options{Restart: envelope.OnFailure, Retry: retry.DefaultOptions},
	}

	// Start the envelope.
	id := uuid.New().String()
	wlet := &protos.WeaveletInfo{
		App:           b.dep.App.Name,
		DeploymentId:  b.dep.Id,
		Group:         b.group,
		GroupId:       id,
		Id:            id,
		SameProcess:   b.dep.App.SameProcess,
		Sections:      b.dep.App.Sections,
		SingleProcess: b.dep.SingleProcess,
	}
	e, err := envelope.NewEnvelope(wlet, b.dep.App, b, b.opts)
	if err != nil {
		return err
	}
	b.envelope = e
	go b.collectMetrics()
	return e.Run(b.ctx)
}

func (b *babysitter) collectMetrics() {
	tickerCollectMetrics := time.NewTicker(time.Minute)
	defer tickerCollectMetrics.Stop()
	for {
		select {
		case <-tickerCollectMetrics.C:
			ms, err := b.envelope.ReadMetrics()
			if err != nil {
				b.logger.Error("Unable to collect metrics", err, "weavelet", b.envelope.Weavelet().Id)
				continue
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
	b.logger.Debug("Starting colocation group", "group", req.Name)
	return protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.mgrAddr,
		URLPath: startColocationGroupURL,
		Request: req,
	})
}

// RegisterReplica implements the protos.EnvelopeHandler interface.
func (b *babysitter) RegisterReplica(replica *protos.ReplicaToRegister) error {
	b.logger.Info("Replica (re)started with new address",
		"group", logging.ShortenComponent(replica.Group),
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
func (b *babysitter) GetAddress(req *protos.GetAddressRequest) (*protos.GetAddressReply, error) {
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	return &protos.GetAddressReply{Address: fmt.Sprintf("%s:0", host)}, nil
}

func (b *babysitter) ExportListener(req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
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
	err := protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.mgrAddr,
		URLPath: recvLogEntryURL,
		Request: req,
	})
	if err != nil {
		b.logger.Error("Error receiving logs", err, "fromAddr", b.mgrAddr)
	}
}

// RecvTraceSpans implements the protos.EnvelopeHandler interface.
func (b *babysitter) RecvTraceSpans(spans []trace.ReadOnlySpan) error {
	return b.traceExporter.ExportSpans(b.ctx, spans)
}
