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
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	imetrics "github.com/ServiceWeaver/weaver/internal/metrics"
	"github.com/ServiceWeaver/weaver/internal/must"
	"github.com/ServiceWeaver/weaver/internal/routing"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/perfetto"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ServiceWeaver/weaver/internal/proto"
	"github.com/ServiceWeaver/weaver/internal/proxy"
	"github.com/ServiceWeaver/weaver/internal/status"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/internal/versioned"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/sdk/trace"
)

const (
	// URL suffixes for various SSH manager handlers.
	getComponentsToStartURL = "/manager/get_components_to_start"
	registerReplicaURL      = "/manager/register_replica"
	exportListenerURL       = "/manager/export_listener"
	startComponentURL       = "/manager/start_component"
	getRoutingInfoURL       = "/manager/get_routing_info"
	recvLogEntryURL         = "/manager/recv_log_entry"
	recvTraceSpansURL       = "/manager/recv_trace_spans"
	recvMetricsURL          = "/manager/recv_metrics"

	// babysitterInfoKey is the name of the env variable that contains deployment
	// information for a babysitter deployed using SSH.
	babysitterInfoKey = "SERVICEWEAVER_BABYSITTER_INFO"
)

var (
	// The directories and files where "weaver ssh" stores data.
	//
	// TODO(mwhittaker): Take these as arguments and move them to ssh.go.
	dataDir      = filepath.Join(must.Must(runtime.DataDir()), "ssh")
	LogDir       = filepath.Join(dataDir, "logs")
	registryDir  = filepath.Join(dataDir, "registry")
	PerfettoFile = filepath.Join(dataDir, "perfetto.db")
)

// manager manages an application version deployment across a set of locations,
// where a location can be a physical or a virtual machine.
//
// TODO(rgrandl): Right now there is a lot of duplicate code between the
// internal/babysitter and the internal/tool/ssh/impl/manager. See if we can reduce the
// duplicated code.
type manager struct {
	ctx        context.Context
	dep        *protos.Deployment
	logger     *slog.Logger
	locations  []string // addresses of the locations
	mgrAddress string   // manager address
	registry   *status.Registry
	started    time.Time

	// keys: addresses of the locations
	// value: tmpdir in which the weaver binary is store
	locs map[string]string

	// logSaver processes log entries generated by the weavelets and babysitters.
	// The entries either have the timestamp produced by the weavelet/babysitter,
	// or have a nil Time field. Defaults to a log saver that pretty prints log
	// entries to stderr.
	//
	// logSaver is called concurrently from multiple goroutines, so it should
	// be thread safe.
	logSaver func(*protos.LogEntry)

	// traceSaver processes trace spans generated by the weavelet. If nil,
	// weavelet traces are dropped.
	//
	// traceSaver is called concurrently from multiple goroutines, so it should
	// be thread safe.
	traceSaver func(spans *protos.TraceSpans) error

	// statsProcessor tracks and computes stats to be rendered on the /statusz page.
	statsProcessor *imetrics.StatsProcessor

	// colocation maps a component to the name of its colocation group. If a
	// component is missing in the map, then it is in a colocation group by
	// itself.
	colocation map[string]string

	mu      sync.Mutex                                    // guards following structures, but not contents
	groups  map[string]*group                             // groups, by group name
	proxies map[string]*proxyInfo                         // proxies, by listener name
	metrics map[groupReplicaInfo][]*protos.MetricSnapshot // latest metrics, by group name and replica id
}

type group struct {
	name       string
	components *versioned.Versioned[map[string]bool] // started components

	mu        sync.Mutex                                           // guards the following
	started   bool                                                 // has this group been started?
	addresses map[string]bool                                      // weavelet addresses
	routings  map[string]*versioned.Versioned[*protos.RoutingInfo] // routing info, by component
	pids      []int64                                              // weavelet pids
}

type proxyInfo struct {
	listener string       // listener name
	proxy    *proxy.Proxy // the proxy
	addr     string       // dialable address of the proxy
}

type groupReplicaInfo struct {
	name string
	id   int32
}

var _ status.Server = &manager{}

// RunManager creates and runs a new manager.
func RunManager(ctx context.Context, dep *protos.Deployment, locations []string) (func() error, error) {
	// Create log saver.
	fs, err := logging.NewFileStore(LogDir)
	if err != nil {
		return nil, fmt.Errorf("cannot create log storage: %w", err)
	}
	logSaver := fs.Add

	logger := slog.New(&logging.LogHandler{
		Opts: logging.Options{
			App:       dep.App.Name,
			Component: "manager",
			Weavelet:  uuid.NewString(),
			Attrs:     []string{"serviceweaver/system", ""},
		},
		Write: logSaver,
	})

	// Create the trace saver.
	traceDB, err := perfetto.Open(ctx, PerfettoFile)
	if err != nil {
		return nil, fmt.Errorf("cannot open Perfetto database: %w", err)
	}
	traceSaver := func(spans *protos.TraceSpans) error {
		var traces []trace.ReadOnlySpan
		for _, span := range spans.Span {
			traces = append(traces, &traceio.ReadSpan{Span: span})
		}
		return traceDB.Store(ctx, dep.App.Name, dep.Id, traces)
	}

	// Form co-location.
	colocation := map[string]string{}
	for _, group := range dep.App.Colocate {
		for _, c := range group.Components {
			colocation[c] = group.Components[0]
		}
	}

	// Create the manager.
	m := &manager{
		ctx:            ctx,
		dep:            dep,
		locations:      locations,
		logger:         logger,
		logSaver:       logSaver,
		traceSaver:     traceSaver,
		statsProcessor: imetrics.NewStatsProcessor(),
		started:        time.Now(),
		colocation:     colocation,
		groups:         map[string]*group{},
		proxies:        map[string]*proxyInfo{},
		metrics:        map[groupReplicaInfo][]*protos.MetricSnapshot{},
	}

	// Run the manager.
	go func() {
		if err := m.run(); err != nil {
			m.logger.Error("Unable to run the manager", "err", err)
		}
	}()

	// Run the stats collector.
	go func() {
		err := m.statsProcessor.CollectMetrics(
			m.ctx, func() []*metrics.MetricSnapshot {

				m.mu.Lock()
				defer m.mu.Unlock()
				var result []*metrics.MetricSnapshot
				for _, ms := range m.metrics {
					for _, m := range ms {
						result = append(result, metrics.UnProto(m))
					}
				}
				return result
			})
		if err != nil {
			m.logger.Error("Unable to collect metrics", "err", err)
		}
	}()

	return func() error {
		return m.registry.Unregister(m.ctx, m.dep.Id)
	}, nil
}

func (m *manager) run() error {
	host, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("manager: get hostname: %v", err)
	}
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:0", host))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	m.mgrAddress = fmt.Sprintf("http://%s", lis.Addr())

	m.logger.Info("Manager listening", "address", m.mgrAddress)

	mux := http.NewServeMux()
	m.addHTTPHandlers(mux)
	m.registerStatusPages(mux)

	go func() {
		if err := serveHTTP(m.ctx, lis, mux); err != nil {
			m.logger.Error("Unable to start HTTP server", "err", err)
		}
	}()

	// Start the main process.
	if err := m.startComponent(m.ctx, &protos.ActivateComponentRequest{
		Component: runtime.Main,
	}); err != nil {
		return err
	}

	// Wait for the status server to become active.
	client := status.NewClient(lis.Addr().String())
	for r := retry.Begin(); r.Continue(m.ctx); {
		_, err := client.Status(m.ctx)
		if err == nil {
			break
		}
		m.logger.Error("Error starting status server", "err", err, "address", lis.Addr())
	}

	// Register the deployment.
	registry, err := DefaultRegistry(m.ctx)
	if err != nil {
		return fmt.Errorf("create registry: %w", err)
	}
	m.registry = registry
	reg := status.Registration{
		DeploymentId: m.dep.Id,
		App:          m.dep.App.Name,
		Addr:         lis.Addr().String(),
	}
	fmt.Fprint(os.Stderr, reg.Rolodex())
	return registry.Register(m.ctx, reg)
}

// addHTTPHandlers adds handlers for the HTTP endpoints exposed by the SSH manager.
func (m *manager) addHTTPHandlers(mux *http.ServeMux) {
	mux.HandleFunc(getComponentsToStartURL, protomsg.HandlerFunc(m.logger, m.getComponentsToStart))
	mux.HandleFunc(registerReplicaURL, protomsg.HandlerDo(m.logger, m.registerReplica))
	mux.HandleFunc(exportListenerURL, protomsg.HandlerFunc(m.logger, m.exportListener))
	mux.HandleFunc(startComponentURL, protomsg.HandlerDo(m.logger, m.startComponent))
	mux.HandleFunc(getRoutingInfoURL, protomsg.HandlerFunc(m.logger, m.getRoutingInfo))
	mux.HandleFunc(recvLogEntryURL, protomsg.HandlerDo(m.logger, m.handleLogEntry))
	mux.HandleFunc(recvTraceSpansURL, protomsg.HandlerDo(m.logger, m.handleTraceSpans))
	mux.HandleFunc(recvMetricsURL, protomsg.HandlerDo(m.logger, m.handleRecvMetrics))
}

// registerStatusPages registers the status pages with the provided mux.
func (m *manager) registerStatusPages(mux *http.ServeMux) {
	status.RegisterServer(mux, m, m.logger)
}

// Status implements the status.Server interface.
//
// TODO(rgrandl): the implementation is the same as the internal/tool/multi/deployer.go.
// See if we can remove duplication.
func (m *manager) Status(ctx context.Context) (*status.Status, error) {
	stats := m.statsProcessor.GetStatsStatusz()
	var components []*status.Component
	for _, g := range m.allGroups() {
		g.components.Lock()
		cs := maps.Keys(g.components.Val)
		g.components.Unlock()
		g.mu.Lock()
		pids := slices.Clone(g.pids)
		g.mu.Unlock()
		for _, component := range cs {
			c := &status.Component{
				Name:  component,
				Group: g.name,
				Pids:  pids,
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
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	var listeners []*status.Listener
	for name, proxy := range m.proxies {
		listeners = append(listeners, &status.Listener{
			Name: name,
			Addr: proxy.addr,
		})
	}
	return &status.Status{
		App:            m.dep.App.Name,
		DeploymentId:   m.dep.Id,
		SubmissionTime: timestamppb.New(m.started),
		Components:     components,
		Listeners:      listeners,
		Config:         m.dep.App,
	}, nil
}

// Metrics implements the status.Server interface.
func (m *manager) Metrics(context.Context) (*status.Metrics, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ms := &status.Metrics{}
	for _, snap := range m.metrics {
		ms.Metrics = append(ms.Metrics, snap...)
	}
	return ms, nil
}

// Profile implements the status.Server interface.
func (m *manager) Profile(context.Context, *protos.GetProfileRequest) (*protos.GetProfileReply, error) {
	return nil, nil
}

// group returns the named co-location group.
//
// REQUIRES: m.mu is not held.
func (m *manager) group(component string) *group {
	m.mu.Lock()
	defer m.mu.Unlock()

	name, ok := m.colocation[component]
	if !ok {
		name = component
	}

	g, ok := m.groups[name]
	if !ok {
		g = &group{
			name:       name,
			addresses:  map[string]bool{},
			components: versioned.Version(map[string]bool{}),
			routings:   map[string]*versioned.Versioned[*protos.RoutingInfo]{},
		}
		m.groups[name] = g
	}
	return g
}

// allAddresses returns a copy of all current addresses in the group.
//
// REQUIRES: g.mu is NOT held.
func (g *group) allAddresses() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return maps.Keys(g.addresses) // creates a new slice.
}

// routing returns the RoutingInfo for the provided component.
//
// REQUIRES: g.mu is NOT held.
func (g *group) routing(component string) *versioned.Versioned[*protos.RoutingInfo] {
	g.mu.Lock()
	defer g.mu.Unlock()
	routing, ok := g.routings[component]
	if !ok {
		routing = versioned.Version(&protos.RoutingInfo{Component: component})
		g.routings[component] = routing
	}
	return routing
}

// allGroups returns all of the managed colocation groups.
func (m *manager) allGroups() []*group {
	m.mu.Lock()
	defer m.mu.Unlock()
	return maps.Values(m.groups) // creates a new slice
}

func (m *manager) getComponentsToStart(_ context.Context, req *GetComponentsRequest) (*GetComponentsReply, error) {
	// TODO(mwhittaker): Right now, this code assumes a group is named after
	// its first component. Update the code to not depend on that assumption.
	g := m.group(req.Group)
	version := g.components.RLock(req.Version)
	defer g.components.RUnlock()
	return &GetComponentsReply{
		Components: maps.Keys(g.components.Val),
		Version:    version,
	}, nil
}

func (m *manager) registerReplica(_ context.Context, req *ReplicaToRegister) error {
	g := m.group(req.Group)

	// Update addresses and pids.
	record := func() bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.addresses[req.Address] {
			// Replica already registered.
			return true
		}
		g.addresses[req.Address] = true
		g.pids = append(g.pids, req.Pid)
		return false
	}
	if record() {
		return nil
	}

	// Update routing.
	replicas := g.allAddresses()
	for _, routing := range g.routings {
		routing.Lock()
		routing.Val.Replicas = replicas
		if routing.Val.Assignment != nil {
			routing.Val.Assignment = routingAlgo(routing.Val.Assignment, replicas)
		}
		routing.Unlock()
	}
	return nil
}

func (m *manager) exportListener(_ context.Context, req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update the proxy.
	if p, ok := m.proxies[req.Listener]; ok {
		p.proxy.AddBackend(req.Address)
		return &protos.ExportListenerReply{ProxyAddress: p.addr}, nil
	}

	lis, err := net.Listen("tcp", req.LocalAddress)
	if errors.Is(err, syscall.EADDRINUSE) {
		// Don't retry if the address is already in use.
		return &protos.ExportListenerReply{Error: err.Error()}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("proxy listen: %w", err)
	}
	addr := lis.Addr().String()
	m.logger.Info("Proxy listening", "address", addr)
	proxy := proxy.NewProxy(m.logger)
	proxy.AddBackend(req.Address)
	m.proxies[req.Listener] = &proxyInfo{
		listener: req.Listener,
		proxy:    proxy,
		addr:     addr,
	}
	go func() {
		if err := serveHTTP(m.ctx, lis, proxy); err != nil {
			m.logger.Error("Proxy", "err", err)
		}
	}()
	return &protos.ExportListenerReply{ProxyAddress: addr}, nil
}

func (m *manager) startComponent(ctx context.Context, req *protos.ActivateComponentRequest) error {
	g := m.group(req.Component)

	// Record the component.
	record := func() bool {
		g.components.Lock()
		defer g.components.Unlock()
		if g.components.Val[req.Component] {
			// Component already started, or is in the process of being started.
			return true
		}
		g.components.Val[req.Component] = true
		return false
	}
	if record() { // already started
		return nil
	}

	// Update the routing info.
	routing := g.routing(req.Component)
	addresses := g.allAddresses()
	update := func() {
		routing.Lock()
		defer routing.Unlock()

		routing.Val.Replicas = addresses
		if req.Routed {
			routing.Val.Assignment = routingAlgo(&protos.Assignment{}, routing.Val.Replicas)
		}
	}
	update()

	// Start the colocation group, if it hasn't already started.
	return m.startColocationGroup(g, req.Component == runtime.Main)
}

// REQUIRES: g.mu is NOT held.
func (m *manager) startColocationGroup(g *group, runMain bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.started {
		// This group has already been started.
		return nil
	}
	g.started = true

	// Start the colocation group. Right now, the number of replicas for each
	// colocation group is equal to the number of locations.
	//
	// TODO(rgrandl): Implement some smarter logic to determine the number of
	// replicas for each group.
	replicaId := 0
	for loc := range m.locations {
		info := &BabysitterInfo{
			ManagerAddr: m.mgrAddress,
			Deployment:  m.dep,
			Group:       g.name,
			ReplicaId:   int32(replicaId),
			LogDir:      LogDir,
			RunMain:     runMain,
		}
		if err := m.startBabysitter(loc, info); err != nil {
			return fmt.Errorf("unable to start babysitter for group %s at location %s: %w\n", g.name, loc, err)
		}
		m.logger.Info("Started babysitter", "location", loc, "colocation group", g.name)
		replicaId++
	}
	return nil
}

func (m *manager) handleLogEntry(_ context.Context, entry *protos.LogEntry) error {
	m.logSaver(entry)
	return nil
}

func (m *manager) handleTraceSpans(_ context.Context, spans *protos.TraceSpans) error {
	if m.traceSaver == nil {
		return nil
	}
	return m.traceSaver(spans)
}

func (m *manager) handleRecvMetrics(_ context.Context, metrics *BabysitterMetrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics[groupReplicaInfo{name: metrics.GroupName, id: metrics.ReplicaId}] = metrics.Metrics
	return nil
}

// startBabysitter starts a new babysitter that manages a colocation group using SSH.
func (m *manager) startBabysitter(loc string, info *BabysitterInfo) error {
	input, err := proto.ToEnv(info)
	if err != nil {
		return err
	}

	env := fmt.Sprintf("%s=%s", babysitterInfoKey, input)
	binaryPath := filepath.Join(m.locations[loc], "weaver")
	cmd := exec.Command("ssh", loc, env, binaryPath, "ssh", "babysitter")
	return cmd.Start()
}

func (m *manager) getRoutingInfo(_ context.Context, req *GetRoutingInfoRequest) (*GetRoutingInfoReply, error) {
	g := m.group(req.RequestingGroup)
	target := m.group(req.Component)

	if !req.Routed && g.name == target.name {
		// Route locally.
		return &GetRoutingInfoReply{
			RoutingInfo: &protos.RoutingInfo{
				Component: req.Component,
				Local:     true,
			},
		}, nil
	}

	routing := target.routing(req.Component)
	version := routing.RLock(req.Version)
	defer routing.RUnlock()
	return &GetRoutingInfoReply{
		RoutingInfo: protomsg.Clone(routing.Val),
		Version:     version,
	}, nil
}

func routingAlgo(currAssignment *protos.Assignment, candidates []string) *protos.Assignment {
	assignment := routing.EqualSlices(candidates)
	assignment.Version = currAssignment.Version + 1
	return assignment
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

// DefaultRegistry returns the default registry in
// $XDG_DATA_HOME/serviceweaver/ssh/registry, or
// ~/.local/share/serviceweaver/ssh/registry if XDG_DATA_HOME is not set.
func DefaultRegistry(ctx context.Context) (*status.Registry, error) {
	return status.NewRegistry(ctx, registryDir)
}
