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
	"github.com/ServiceWeaver/weaver/internal/logtype"
	imetrics "github.com/ServiceWeaver/weaver/internal/metrics"
	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/internal/status"
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
	"google.golang.org/protobuf/types/known/timestamppb"
)

// singleprocessEnv implements the env used for singleprocess Service Weaver applications.
type singleprocessEnv struct {
	ctx      context.Context
	weavelet *protos.WeaveletInfo
	config   *protos.AppConfig

	submissionTime time.Time
	statsProcessor *imetrics.StatsProcessor // tracks and computes stats to be rendered on the /statusz page.
	traceSaver     func(spans *protos.Spans) error

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

	wlet := &protos.WeaveletInfo{
		App:                appConfig.Name,
		DeploymentId:       uuid.New().String(),
		Group:              &protos.ColocationGroup{Name: "main"},
		GroupId:            uuid.New().String(),
		Id:                 uuid.New().String(),
		SameProcess:        appConfig.SameProcess,
		Sections:           appConfig.Sections,
		SingleProcess:      true,
		UseLocalhost:       true,
		WeaveletPicksPorts: true,
	}
	if err := runtime.CheckWeaveletInfo(wlet); err != nil {
		return nil, err
	}

	traceDB, err := perfetto.Open(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot open Perfetto database: %w", err)
	}
	traceSaver := func(spans *protos.Spans) error {
		traces := make([]sdktrace.ReadOnlySpan, len(spans.Span))
		for i, span := range spans.Span {
			traces[i] = &traceio.ReadSpan{Span: span}
		}
		return traceDB.Store(ctx, appConfig.Name, wlet.DeploymentId, traces)
	}

	env := &singleprocessEnv{
		ctx:            ctx,
		weavelet:       wlet,
		config:         appConfig,
		submissionTime: time.Now(),
		listeners:      map[string][]string{},
		statsProcessor: imetrics.NewStatsProcessor(),
		traceSaver:     traceSaver,
	}
	go env.statsProcessor.CollectMetrics(ctx, metrics.Snapshot)
	return env, nil
}

func (e *singleprocessEnv) GetWeaveletInfo() *protos.WeaveletInfo {
	return e.weavelet
}

func (e *singleprocessEnv) StartColocationGroup(context.Context, *protos.ColocationGroup) error {
	// All processes are hosted in this colocation group, so we do not support starting colocation groups.
	return fmt.Errorf("cannot start other colocation groups for a singleprocess execution")
}

func (e *singleprocessEnv) RegisterComponentToStart(_ context.Context, _ string, name string, _ bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.components = append(e.components, name)
	return nil
}

func (e *singleprocessEnv) GetComponentsToStart(ctx context.Context, _ *call.Version) ([]string, *call.Version, error) {
	// Block forever since there's not going to be anything to start.
	<-ctx.Done()
	return []string{}, nil, nil
}

func (e *singleprocessEnv) RegisterReplica(context.Context, call.NetworkAddress) error {
	return nil
}

func (e *singleprocessEnv) ReportLoad(context.Context, *protos.WeaveletLoadReport) error {
	return nil
}

func (e *singleprocessEnv) GetRoutingInfo(context.Context, string, *call.Version) (*protos.RoutingInfo, *call.Version, error) {
	return nil, nil, fmt.Errorf("routing info not useful for singleprocess execution")
}

func (e *singleprocessEnv) GetAddress(_ context.Context, listener string, opts ListenerOptions) (*protos.GetAddressReply, error) {
	return &protos.GetAddressReply{Address: opts.LocalAddress}, nil
}

func (e *singleprocessEnv) ExportListener(_ context.Context, lis *protos.Listener, opts ListenerOptions) (*protos.ExportListenerReply, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners[lis.Name] = append(e.listeners[lis.Name], lis.Addr)
	return &protos.ExportListenerReply{}, nil
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
		e.SystemLogger().Error("status server unavailable", err, "address", lis.Addr())
	}

	// Register the deployment.
	dir, err := status.DefaultRegistryDir()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, "single_registry")
	registry, err := status.NewRegistry(ctx, dir)
	if err != nil {
		return nil
	}
	reg := status.Registration{
		DeploymentId: e.weavelet.DeploymentId,
		App:          e.weavelet.App,
		Addr:         lis.Addr().String(),
	}
	fmt.Fprint(os.Stderr, reg.Rolodex())
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
		App:            e.weavelet.App,
		DeploymentId:   e.weavelet.DeploymentId,
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
		proto.Labels["serviceweaver_app"] = e.weavelet.App
		proto.Labels["serviceweaver_version"] = e.weavelet.DeploymentId
		proto.Labels["serviceweaver_node"] = e.weavelet.Id
		m.Metrics = append(m.Metrics, proto)
	}
	return m, nil
}

// Profile implements the status.Server interface.
func (e *singleprocessEnv) Profile(_ context.Context, req *protos.RunProfiling) (*protos.Profile, error) {
	data, err := conn.Profile(req)
	profile := &protos.Profile{
		AppName:   e.weavelet.App,
		VersionId: e.weavelet.DeploymentId,
		Data:      data,
	}
	if err != nil {
		profile.Errors = []string{err.Error()}
	}
	return profile, nil
}

func (e *singleprocessEnv) CreateLogSaver(_ context.Context, component string) func(entry *protos.LogEntry) {
	pp := logging.NewPrettyPrinter(colors.Enabled())
	return func(entry *protos.LogEntry) {
		fmt.Fprintln(os.Stderr, pp.Format(entry))
	}
}

func (e *singleprocessEnv) CreateTraceExporter() (sdktrace.SpanExporter, error) {
	return traceio.NewWriter(e.traceSaver), nil
}

func (e *singleprocessEnv) SystemLogger() logtype.Logger {
	// In single process execution, system logs are hidden.
	return discardingLogger{}
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
