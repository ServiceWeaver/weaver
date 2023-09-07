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

package weaver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/ServiceWeaver/weaver/internal/config"
	"github.com/ServiceWeaver/weaver/internal/env"
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
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"github.com/ServiceWeaver/weaver/runtime/traces"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SingleWeaveletOptions configure a SingleWeavelet.
type SingleWeaveletOptions struct {
	ConfigFilename string               // TOML config filename
	Config         string               // TOML config contents
	Fakes          map[reflect.Type]any // component fakes, by component interface type
	Quiet          bool                 // if true, do not print or log anything
}

// SingleWeavelet is a weavelet that runs all components locally in a single
// process. It is the weavelet used when you "go run" a Service Weaver app.
type SingleWeavelet struct {
	// Registrations.
	regs       []*codegen.Registration                // registered components
	regsByName map[string]*codegen.Registration       // registrations by component name
	regsByIntf map[reflect.Type]*codegen.Registration // registrations by component interface type
	regsByImpl map[reflect.Type]*codegen.Registration // registrations by component implementation type

	// Options, config, and metadata.
	opts         SingleWeaveletOptions // options
	config       *single.SingleConfig  // "[single]" section of config file
	deploymentId string                // globally unique deployment id
	id           string                // globally unique weavelet id
	createdAt    time.Time             // time at which the weavelet was created

	// Logging, tracing, and metrics.
	pp     *logging.PrettyPrinter   // pretty printer for logger
	tracer trace.Tracer             // tracer used by all components
	stats  *imetrics.StatsProcessor // metrics aggregator

	// Components and listeners.
	mu         sync.Mutex              // guards the following fields
	components map[string]any          // components, by name
	listeners  map[string]net.Listener // listeners, by name
}

// NewSingleWeavelet returns a new SingleWeavelet that hosts the components
// specified in the provided registrations.
func NewSingleWeavelet(ctx context.Context, regs []*codegen.Registration, opts SingleWeaveletOptions) (*SingleWeavelet, error) {
	// Parse config.
	config, err := parseSingleConfig(regs, opts.ConfigFilename, opts.Config)
	if err != nil {
		return nil, err
	}
	env, err := env.Parse(config.App.Env)
	if err != nil {
		return nil, err
	}
	for k, v := range env {
		if err := os.Setenv(k, v); err != nil {
			return nil, err
		}
	}

	// Set up tracer.
	deploymentId := uuid.New().String()
	id := uuid.New().String()
	tracer, err := singleTracer(ctx, config.App.Name, deploymentId, id)
	if err != nil {
		return nil, err
	}

	// Index registrations.
	regsByName := map[string]*codegen.Registration{}
	regsByIntf := map[reflect.Type]*codegen.Registration{}
	regsByImpl := map[reflect.Type]*codegen.Registration{}
	for _, reg := range regs {
		regsByName[reg.Name] = reg
		regsByIntf[reg.Iface] = reg
		regsByImpl[reg.Impl] = reg
	}

	// Print rolodex card.
	//
	// TODO(mwhittaker): We may not want to print this out. Revisit.
	if !opts.Quiet {
		reg := status.Registration{
			DeploymentId: deploymentId,
			App:          config.App.Name,
		}
		fmt.Fprint(os.Stderr, reg.Rolodex())
	}

	return &SingleWeavelet{
		regs:         regs,
		regsByName:   regsByName,
		regsByIntf:   regsByIntf,
		regsByImpl:   regsByImpl,
		opts:         opts,
		config:       config,
		deploymentId: deploymentId,
		id:           id,
		createdAt:    time.Now(),
		pp:           logging.NewPrettyPrinter(colors.Enabled()),
		tracer:       tracer,
		stats:        imetrics.NewStatsProcessor(),
		components:   map[string]any{},
		listeners:    map[string]net.Listener{},
	}, nil
}

// parseSingleConfig parses the "[single]" section of a config file.
func parseSingleConfig(regs []*codegen.Registration, filename, contents string) (*single.SingleConfig, error) {
	// Parse the config file, if one is given.
	config := &single.SingleConfig{App: &protos.AppConfig{}}
	if contents != "" {
		app, err := runtime.ParseConfig(filename, contents, codegen.ComponentConfigValidator)
		if err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
		if err := runtime.ParseConfigSection(single.ConfigKey, single.ShortConfigKey, app.Sections, config); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
		config.App = app
	}
	if config.App.Name == "" {
		config.App.Name = filepath.Base(os.Args[0])
	}

	// Validate listeners in the config.
	listeners := map[string]struct{}{}
	for _, reg := range regs {
		for _, listener := range reg.Listeners {
			listeners[listener] = struct{}{}
		}
	}
	for listener := range config.Listeners {
		if _, ok := listeners[listener]; !ok {
			return nil, fmt.Errorf("listener %s (in the config) not found", listener)
		}
	}

	return config, nil
}

// singleTracer returns a tracer for single process execution.
func singleTracer(ctx context.Context, app, deploymentId, id string) (trace.Tracer, error) {
	traceDB, err := traces.OpenDB(ctx, single.PerfettoFile)
	if err != nil {
		return nil, fmt.Errorf("cannot open Perfetto database: %w", err)
	}
	exporter := traceio.NewWriter(func(spans *protos.TraceSpans) error {
		return traceDB.Store(ctx, app, deploymentId, spans)
	})
	return tracer(exporter, app, deploymentId, id), nil
}

// GetIntf implements the Weavelet interface.
func (w *SingleWeavelet) GetIntf(t reflect.Type) (any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.getIntf(t, "root")
}

// GetImpl implements the Weavelet interface.
func (w *SingleWeavelet) GetImpl(t reflect.Type) (any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.getImpl(t)
}

// getIntf returns the component with the provided interface type. The returned
// value has type t.
//
// REQUIRES: w.mu is held.
func (w *SingleWeavelet) getIntf(t reflect.Type, requester string) (any, error) {
	reg, ok := w.regsByIntf[t]
	if !ok {
		return nil, fmt.Errorf("component %v not found; maybe you forgot to run weaver generate", t)
	}
	c, err := w.get(reg)
	if err != nil {
		return nil, err
	}
	return reg.LocalStubFn(c, requester, w.tracer), nil
}

// getImpl returns the component with the provided implementation type. The
// returned value has type t.
//
// REQUIRES: w.mu is held.
func (w *SingleWeavelet) getImpl(t reflect.Type) (any, error) {
	reg, ok := w.regsByImpl[t]
	if !ok {
		return nil, fmt.Errorf("component implementation %v not found; maybe you forgot to run weaver generate", t)
	}
	return w.get(reg)
}

// get returns the component with the provided registration, creating it if
// necessary.
//
// REQUIRES: w.mu is held.
func (w *SingleWeavelet) get(reg *codegen.Registration) (any, error) {
	if c, ok := w.components[reg.Name]; ok {
		// The component has already been created.
		return c, nil
	}

	if fake, ok := w.opts.Fakes[reg.Iface]; ok {
		// We have a fake registered for this component.
		return fake, nil
	}

	// Create the component implementation.
	v := reflect.New(reg.Impl)
	obj := v.Interface()

	// Fill config.
	if cfg := config.Config(v); cfg != nil {
		if err := runtime.ParseConfigSection(reg.Name, "", w.config.App.Sections, cfg); err != nil {
			return nil, err
		}
	}

	// Set logger.
	if err := SetLogger(obj, w.logger(reg.Name)); err != nil {
		return nil, err
	}

	// Fill ref fields.
	if err := FillRefs(obj, func(t reflect.Type) (any, error) {
		return w.getIntf(t, reg.Name)
	}); err != nil {
		return nil, err
	}

	// Fill listener fields.
	if err := FillListeners(obj, func(name string) (net.Listener, string, error) {
		lis, err := w.listener(name)
		return lis, "", err
	}); err != nil {
		return nil, err
	}

	// Call Init if available.
	if i, ok := obj.(interface{ Init(context.Context) error }); ok {
		// TODO(mwhittaker): Use better context.
		if err := i.Init(context.Background()); err != nil {
			return nil, fmt.Errorf("component %q initialization failed: %w", reg.Name, err)
		}
	}

	w.components[reg.Name] = obj
	return obj, nil
}

// listener returns the listener with the provided name.
//
// REQUIRES: w.mu is held.
func (w *SingleWeavelet) listener(name string) (net.Listener, error) {
	if lis, ok := w.listeners[name]; ok {
		// The listener already exists.
		return lis, nil
	}

	// Create the listener.
	var addr string
	if opts, ok := w.config.Listeners[name]; ok {
		addr = opts.Address
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	// Store the listener.
	w.listeners[name] = lis
	return lis, err
}

// logger returns a logger for the component with the provided name.
func (w *SingleWeavelet) logger(name string) *slog.Logger {
	write := func(entry *protos.LogEntry) {
		msg := w.pp.Format(entry)
		if w.opts.Quiet {
			// Note that we format the log entry regardless of whether we print
			// it so that benchmark results are not skewed significantly by the
			// presence of the -test.v flag.
			return
		}
		fmt.Fprintln(os.Stderr, msg)
	}
	return slog.New(&logging.LogHandler{
		Opts: logging.Options{
			App:        w.config.App.Name,
			Deployment: w.deploymentId,
			Component:  name,
			Weavelet:   w.id,
		},
		Write: write,
	})
}

// ServeStatus runs an HTTP status server.
func (w *SingleWeavelet) ServeStatus(ctx context.Context) error {
	// Single process deployments don't produce system logs.
	noopLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError + 1}))

	// Launch the stats processor.
	go func() {
		err := w.stats.CollectMetrics(ctx, metrics.Snapshot)
		if err != nil {
			noopLogger.Error("metric collection stopped with error", "err", err)
		}
	}()

	// Start a signal handler to detect when the process is killed.
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	// Spawn the status server.
	mux := http.NewServeMux()
	mux.Handle("/debug/pprof/", http.DefaultServeMux)
	status.RegisterServer(mux, w, noopLogger)
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
		noopLogger.Error("status server unavailable", "err", err, "address", lis.Addr())
	}

	// Register the deployment.
	registry, err := status.NewRegistry(ctx, single.RegistryDir)
	if err != nil {
		return nil
	}
	reg := status.Registration{
		DeploymentId: w.deploymentId,
		App:          w.config.App.Name,
		Addr:         lis.Addr().String(),
	}
	if err := registry.Register(ctx, reg); err != nil {
		return err
	}

	// Unregister the deployment if the HTTP server fails or if the application
	// is killed.
	select {
	case err := <-errs:
		if err := registry.Unregister(ctx, reg.DeploymentId); err != nil {
			fmt.Fprintf(os.Stderr, "unregister deployment: %v\n", err)
		}
		return err
	case <-done:
		code := 0
		if err := registry.Unregister(ctx, reg.DeploymentId); err != nil {
			fmt.Fprintf(os.Stderr, "unregister deployment: %v\n", err)
			code = 1
		}
		os.Exit(code)
	}
	panic("unreachable")
}

// Status implements the status.Server interface.
func (w *SingleWeavelet) Status(context.Context) (*status.Status, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	pid := int64(os.Getpid())
	stats := w.stats.GetStatsStatusz()
	var components []*status.Component
	for component := range w.components {
		c := &status.Component{
			Name: component,
			Pids: []int64{pid},
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

	var listeners []*status.Listener
	for name, lis := range w.listeners {
		listeners = append(listeners, &status.Listener{
			Name: name,
			Addr: lis.Addr().String(),
		})
	}

	return &status.Status{
		App:            w.config.App.Name,
		DeploymentId:   w.deploymentId,
		SubmissionTime: timestamppb.New(w.createdAt),
		Components:     components,
		Listeners:      listeners,
		Config:         w.config.App,
	}, nil
}

// Metrics implements the status.Server interface.
func (w *SingleWeavelet) Metrics(context.Context) (*status.Metrics, error) {
	m := &status.Metrics{}
	for _, snap := range metrics.Snapshot() {
		proto := snap.ToProto()
		if proto.Labels == nil {
			proto.Labels = map[string]string{}
		}
		proto.Labels["serviceweaver_app"] = w.config.App.Name
		proto.Labels["serviceweaver_version"] = w.deploymentId
		proto.Labels["serviceweaver_node"] = w.id
		m.Metrics = append(m.Metrics, proto)
	}
	return m, nil
}

// Profile implements the status.Server interface.
func (w *SingleWeavelet) Profile(_ context.Context, req *protos.GetProfileRequest) (*protos.GetProfileReply, error) {
	data, err := conn.Profile(req)
	return &protos.GetProfileReply{Data: data}, err
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
