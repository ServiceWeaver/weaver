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

package weaver

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	imetrics "github.com/ServiceWeaver/weaver/internal/metrics"
	"github.com/ServiceWeaver/weaver/internal/status"
	"github.com/ServiceWeaver/weaver/internal/tool/single"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/perfetto"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/exp/slog"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// singleprocessEnv implements the env used for singleprocess Service Weaver applications.
type singleprocessEnv struct {
	ctx       context.Context
	bootstrap runtime.Bootstrap
	info      *protos.EnvelopeInfo
	config    *protos.AppConfig

	submissionTime time.Time
	statsProcessor *imetrics.StatsProcessor // tracks and computes stats to be rendered on the /statusz page.
	traceSaver     func(spans *protos.TraceSpans) error
	pp             *logging.PrettyPrinter

	mu         sync.Mutex
	listeners  map[string][]string // listener addresses, keyed by name
	components []string            // list of active components
}

var _ env = &singleprocessEnv{}

func newSingleprocessEnv(bootstrap runtime.Bootstrap) (*singleprocessEnv, error) {
	ctx := context.Background()

	// Get the config to use.
	configFile := "[testconfig]"
	config := bootstrap.TestConfig
	if config == "" {
		// Try to read from the file named by SERVICEWEAVER_CONFIG
		configFile = os.Getenv("SERVICEWEAVER_CONFIG")
		if configFile != "" {
			contents, err := os.ReadFile(configFile)
			if err != nil {
				return nil, fmt.Errorf("config file: %w", err)
			}
			config = string(contents)
		}
	}

	appConfig := &protos.AppConfig{}
	if config != "" {
		var err error
		appConfig, err = runtime.ParseConfig(configFile, config, codegen.ComponentConfigValidator)
		if err != nil {
			return nil, err
		}
	}

	// Overwrite app config with the true command line used.
	appConfig.Name = filepath.Base(os.Args[0])
	appConfig.Binary = os.Args[0]
	appConfig.Args = os.Args[1:]

	wlet := &protos.EnvelopeInfo{
		App:           appConfig.Name,
		DeploymentId:  uuid.New().String(),
		Id:            uuid.New().String(),
		Sections:      appConfig.Sections,
		SingleProcess: true,
		SingleMachine: true,
		RunMain:       true,
	}
	if err := runtime.CheckEnvelopeInfo(wlet); err != nil {
		return nil, err
	}

	traceDB, err := perfetto.Open(ctx, single.PerfettoFile)
	if err != nil {
		return nil, fmt.Errorf("cannot open Perfetto database: %w", err)
	}
	traceSaver := func(spans *protos.TraceSpans) error {
		traces := make([]sdktrace.ReadOnlySpan, len(spans.Span))
		for i, span := range spans.Span {
			traces[i] = &traceio.ReadSpan{Span: span}
		}
		return traceDB.Store(ctx, appConfig.Name, wlet.DeploymentId, traces)
	}

	env := &singleprocessEnv{
		ctx:            ctx,
		bootstrap:      bootstrap,
		info:           wlet,
		config:         appConfig,
		submissionTime: time.Now(),
		listeners:      map[string][]string{},
		statsProcessor: imetrics.NewStatsProcessor(),
		traceSaver:     traceSaver,
		pp:             logging.NewPrettyPrinter(colors.Enabled()),
	}
	go func() {
		err := env.statsProcessor.CollectMetrics(ctx, metrics.Snapshot)
		if err != nil {
			env.SystemLogger().Error("metric collection stopped with error", "err", err)
		}
	}()
	return env, nil
}

func (e *singleprocessEnv) EnvelopeInfo() *protos.EnvelopeInfo {
	return e.info
}

func (e *singleprocessEnv) ActivateComponent(_ context.Context, component string, _ bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.components = append(e.components, component)
	return nil
}

func (e *singleprocessEnv) GetListenerAddress(_ context.Context, listener string, opts ListenerOptions) (*protos.GetListenerAddressReply, error) {
	return &protos.GetListenerAddressReply{Address: opts.LocalAddress}, nil
}

func (e *singleprocessEnv) ExportListener(_ context.Context, listener, addr string, opts ListenerOptions) (*protos.ExportListenerReply, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners[listener] = append(e.listeners[listener], addr)
	return &protos.ExportListenerReply{}, nil
}

func (e *singleprocessEnv) GetSelfCertificate(ctx context.Context) ([]byte, []byte, error) {
	panic("unused")
}

func (e *singleprocessEnv) VerifyClientCertificate(context.Context, [][]byte) ([]string, error) {
	panic("unused")
}

func (e *singleprocessEnv) VerifyServerCertificate(context.Context, [][]byte, string) error {
	panic("unused")
}

// serveStatus runs and registers the weaver-single status server.
func (e *singleprocessEnv) serveStatus(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/debug/pprof/", http.DefaultServeMux)
	status.RegisterServer(mux, e, e.SystemLogger())
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return err
	}
	errs := make(chan error, 1)
	go func() {
		errs <- serveHTTP(ctx, lis, mux)
	}()

	// Wait for the status server to become active.
	client := status.NewClient(lis.Addr().String())
	for r := retry.Begin(); r.Continue(ctx); {
		_, err := client.Status(ctx)
		if err == nil {
			break
		}
		e.SystemLogger().Error("status server unavailable", "err", err, "address", lis.Addr())
	}

	// Register the deployment.
	registry, err := status.NewRegistry(ctx, single.RegistryDir)
	if err != nil {
		return nil
	}
	reg := status.Registration{
		DeploymentId: e.info.DeploymentId,
		App:          e.info.App,
		Addr:         lis.Addr().String(),
	}
	if !e.bootstrap.Quiet {
		fmt.Fprint(os.Stderr, reg.Rolodex())
	}
	if err := registry.Register(ctx, reg); err != nil {
		return err
	}

	// Unregister the deployment if this application is killed.
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-done
		if err := registry.Unregister(ctx, reg.DeploymentId); err != nil {
			fmt.Fprintf(os.Stderr, "unregister deployment: %v\n", err)
		}
		os.Exit(1)
	}()

	return <-errs
}

// Status implements the status.Server interface.
func (e *singleprocessEnv) Status(ctx context.Context) (*status.Status, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// TODO(mwhittaker): The main process should probably be registered like
	// any other process?
	pid := int64(os.Getpid())
	stats := e.statsProcessor.GetStatsStatusz()
	components := []*status.Component{{Name: "main", Pids: []int64{pid}}}
	for _, component := range e.components {
		c := &status.Component{
			Name:  component,
			Group: "main",
			Pids:  []int64{pid},
		}
		components = append(components, c)

		// TODO(mwhittaker): Unify with ui package and remove duplication.
		s := stats[logging.ShortenComponent(component)]
		if s == nil {
			continue
		}
		for _, methodStats := range s {
			c.Methods = append(c.Methods, &status.Method{
				Name: methodStats.Name,
				Minute: &status.MethodStats{
					NumCalls:     methodStats.Minute.NumCalls,
					AvgLatencyMs: methodStats.Minute.AvgLatencyMs,
					RecvKbPerSec: methodStats.Minute.RecvKBPerSec,
					SentKbPerSec: methodStats.Minute.SentKBPerSec,
				},
				Hour: &status.MethodStats{
					NumCalls:     methodStats.Hour.NumCalls,
					AvgLatencyMs: methodStats.Hour.AvgLatencyMs,
					RecvKbPerSec: methodStats.Hour.RecvKBPerSec,
					SentKbPerSec: methodStats.Hour.SentKBPerSec,
				},
				Total: &status.MethodStats{
					NumCalls:     methodStats.Total.NumCalls,
					AvgLatencyMs: methodStats.Total.AvgLatencyMs,
					RecvKbPerSec: methodStats.Total.RecvKBPerSec,
					SentKbPerSec: methodStats.Total.SentKBPerSec,
				},
			})
		}
	}

	// TODO(mwhittaker): Why are there multiple listener addresses?
	var listeners []*status.Listener
	for name, addrs := range e.listeners {
		listeners = append(listeners, &status.Listener{
			Name: name,
			Addr: addrs[0],
		})
	}

	return &status.Status{
		App:            e.info.App,
		DeploymentId:   e.info.DeploymentId,
		SubmissionTime: timestamppb.New(e.submissionTime),
		Components:     components,
		Listeners:      listeners,
		Config:         e.config,
	}, nil
}

// Metrics implements the status.Server interface.
func (e *singleprocessEnv) Metrics(context.Context) (*status.Metrics, error) {
	m := &status.Metrics{}
	for _, snap := range metrics.Snapshot() {
		proto := snap.ToProto()
		if proto.Labels == nil {
			proto.Labels = map[string]string{}
		}
		proto.Labels["serviceweaver_app"] = e.info.App
		proto.Labels["serviceweaver_version"] = e.info.DeploymentId
		proto.Labels["serviceweaver_node"] = e.info.Id
		m.Metrics = append(m.Metrics, proto)
	}
	return m, nil
}

// Profile implements the status.Server interface.
func (e *singleprocessEnv) Profile(_ context.Context, req *protos.GetProfileRequest) (*protos.GetProfileReply, error) {
	data, err := conn.Profile(req)
	return &protos.GetProfileReply{Data: data}, err
}

func (e *singleprocessEnv) CreateLogSaver() func(entry *protos.LogEntry) {
	return func(entry *protos.LogEntry) {
		msg := e.pp.Format(entry)
		if e.bootstrap.Quiet {
			// Note that we format the log entry regardless of whether we print
			// it so that benchmark results are not skewed significantly by the
			// presence of the -test.v flag.
			return
		}
		fmt.Fprintln(os.Stderr, msg)
	}
}

func (e *singleprocessEnv) CreateTraceExporter() sdktrace.SpanExporter {
	return traceio.NewWriter(e.traceSaver)
}

func (e *singleprocessEnv) SystemLogger() *slog.Logger {
	// In single process execution, system logs are hidden.
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// serveHTTP serves HTTP traffic on the provided listener using the provided
// handler. The server is shut down when then provided context is cancelled.
func serveHTTP(ctx context.Context, lis net.Listener, handler http.Handler) error {
	server := http.Server{Handler: handler}
	errs := make(chan error, 1)
	go func() { errs <- server.Serve(lis) }()
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		return server.Shutdown(ctx)
	}
}
