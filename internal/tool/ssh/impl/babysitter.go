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
	"golang.org/x/exp/slog"
)

// babysitter starts and manages weavelets belonging to a single colocation
// group for a single application version, on the local machine.
type babysitter struct {
	ctx           context.Context
	info          *BabysitterInfo
	logger        *slog.Logger
	traceExporter *traceio.Writer // to export traces to the manager
	envelope      *envelope.Envelope

	mu                  sync.Mutex
	watchingRoutingInfo map[string]bool
}

var _ envelope.EnvelopeHandler = &babysitter{}

// RunBabysitter creates and runs an envelope.Envelope and a metrics collector for a
// weavelet deployed with SSH.
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

	id := uuid.New().String()
	b := &babysitter{
		ctx:  ctx,
		info: info,
		logger: slog.New(&logging.LogHandler{
			Opts: logging.Options{
				App:        info.Deployment.App.Name,
				Deployment: info.Deployment.Id,
				Component:  "Babysitter",
				Weavelet:   uuid.NewString(),
				Attrs:      []string{"serviceweaver/system", "", "weavelet", id},
			},
			Write: logSaver,
		}),
		traceExporter: traceio.NewWriter(func(spans *protos.TraceSpans) error {
			return protomsg.Call(ctx, protomsg.CallArgs{
				Client:  http.DefaultClient,
				Addr:    info.ManagerAddr,
				URLPath: recvTraceSpansURL,
				Request: spans,
			})
		}),
		watchingRoutingInfo: map[string]bool{},
	}

	// Start the envelope.
	wlet := &protos.EnvelopeInfo{
		App:           info.Deployment.App.Name,
		DeploymentId:  info.Deployment.Id,
		Id:            id,
		Sections:      info.Deployment.App.Sections,
		SingleProcess: info.Deployment.SingleProcess,
		RunMain:       info.RunMain,
	}
	e, err := envelope.NewEnvelope(ctx, wlet, info.Deployment.App)
	if err != nil {
		return err
	}
	b.envelope = e

	// Make sure the version of the deployer matches the version of the
	// compiled binary.
	winfo := e.WeaveletInfo()

	if err := b.registerReplica(winfo); err != nil {
		return err
	}
	c := metricsCollector{logger: b.logger, envelope: e, info: info}
	go c.run(ctx)
	return e.Serve(b)
}

type metricsCollector struct {
	logger   *slog.Logger
	envelope *envelope.Envelope
	info     *BabysitterInfo
}

func (m *metricsCollector) run(ctx context.Context) {
	tickerCollectMetrics := time.NewTicker(time.Minute)
	defer tickerCollectMetrics.Stop()
	for {
		select {
		case <-tickerCollectMetrics.C:
			ms, err := m.envelope.GetMetrics()
			if err != nil {
				m.logger.Error("Unable to collect metrics", "err", err)
				continue
			}
			ms = append(ms, metrics.Snapshot()...)

			// Convert metrics to metrics protos.
			var metrics []*protos.MetricSnapshot
			for _, m := range ms {
				metrics = append(metrics, m.ToProto())
			}
			if err := protomsg.Call(ctx, protomsg.CallArgs{
				Client:  http.DefaultClient,
				Addr:    m.info.ManagerAddr,
				URLPath: recvMetricsURL,
				Request: &BabysitterMetrics{
					GroupName: m.info.Group,
					ReplicaId: m.info.ReplicaId,
					Metrics:   metrics,
				},
			}); err != nil {
				m.logger.Error("Error collecting metrics", "err", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// ActivateComponent implements the protos.EnvelopeHandler interface.
func (b *babysitter) ActivateComponent(_ context.Context, req *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error) {
	if err := protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.info.ManagerAddr,
		URLPath: startComponentURL,
		Request: req,
	}); err != nil {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.watchingRoutingInfo[req.Component] {
		b.watchingRoutingInfo[req.Component] = true
		go b.watchRoutingInfo(req.Component, req.Routed)
	}
	return &protos.ActivateComponentReply{}, nil
}

// registerReplica registers the information about a colocation group replica
// (i.e., a weavelet).
func (b *babysitter) registerReplica(info *protos.WeaveletInfo) error {
	if err := protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.info.ManagerAddr,
		URLPath: registerReplicaURL,
		Request: &ReplicaToRegister{
			Group:   b.info.Group,
			Address: info.DialAddr,
			Pid:     info.Pid,
		},
	}); err != nil {
		return err
	}

	go b.watchComponents()
	return nil
}

// GetListenerAddress implements the protos.EnvelopeHandler interface.
func (b *babysitter) GetListenerAddress(_ context.Context, req *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	return &protos.GetListenerAddressReply{Address: fmt.Sprintf("%s:0", host)}, nil
}

// ExportListener implements the protos.EnvelopeHandler interface.
func (b *babysitter) ExportListener(_ context.Context, req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	reply := &protos.ExportListenerReply{}
	if err := protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.info.ManagerAddr,
		URLPath: exportListenerURL,
		Request: req,
		Reply:   reply,
	}); err != nil {
		return nil, err
	}
	return reply, nil
}

// VerifyClientCertificate implements the envelope.EnvelopeHandler interface.
func (b *babysitter) VerifyClientCertificate(context.Context, *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error) {
	// TODO(spetrovic): Implement this functionality.
	panic("unimplemented")
}

// VerifyServerCertificate implements the envelope.EnvelopeHandler interface.
func (b *babysitter) VerifyServerCertificate(context.Context, *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error) {
	// TODO(spetrovic): Implement this functionality.
	panic("unimplemented")
}

func (b *babysitter) getRoutingInfo(component string, routed bool, version string) (*protos.RoutingInfo, string, error) {
	req := &GetRoutingInfoRequest{
		RequestingGroup: b.info.Group,
		Component:       component,
		Routed:          routed,
		Version:         version,
	}
	reply := &GetRoutingInfoReply{}
	if err := protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.info.ManagerAddr,
		URLPath: getRoutingInfoURL,
		Request: req,
		Reply:   reply,
	}); err != nil {
		return nil, "", err
	}
	return reply.RoutingInfo, reply.Version, nil
}

func (b *babysitter) watchRoutingInfo(component string, routed bool) {
	version := ""
	for r := retry.Begin(); r.Continue(b.ctx); {
		routing, newVersion, err := b.getRoutingInfo(component, routed, version)
		if err != nil {
			b.logger.Error("cannot get routing info; will retry", "err", err, "component", component)
			continue
		}
		version = newVersion
		if err := b.envelope.UpdateRoutingInfo(routing); err != nil {
			b.logger.Error("cannot update routing info; will retry", "err", err, "component", component)
			continue
		}
		if routing.Local {
			// If the routing is local, it will never change. There is no need
			// to watch.
			return
		}
		r.Reset()
	}
}

func (b *babysitter) getComponentsToStart(version string) ([]string, string, error) {
	req := &GetComponentsRequest{Group: b.info.Group, Version: version}
	reply := &GetComponentsReply{}
	if err := protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.info.ManagerAddr,
		URLPath: getComponentsToStartURL,
		Request: req,
		Reply:   reply,
	}); err != nil {
		return nil, "", err
	}
	return reply.Components, reply.Version, nil
}

func (b *babysitter) watchComponents() {
	version := ""
	for r := retry.Begin(); r.Continue(b.ctx); {
		components, newVersion, err := b.getComponentsToStart(version)
		if err != nil {
			b.logger.Error("cannot get components to start; will retry", "err", err)
			continue
		}
		version = newVersion
		if err := b.envelope.UpdateComponents(components); err != nil {
			b.logger.Error("cannot update components to start; will retry", "err", err)
			continue
		}
		r.Reset()
	}
}

// HandleLogEntry implements the protos.EnvelopeHandler interface.
func (b *babysitter) HandleLogEntry(_ context.Context, req *protos.LogEntry) error {
	return protomsg.Call(b.ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    b.info.ManagerAddr,
		URLPath: recvLogEntryURL,
		Request: req,
	})
}

// HandleTraceSpans implements the protos.EnvelopeHandler interface.
func (b *babysitter) HandleTraceSpans(_ context.Context, spans []trace.ReadOnlySpan) error {
	return b.traceExporter.ExportSpans(b.ctx, spans)
}
