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

package multi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"

	imetrics "github.com/ServiceWeaver/weaver/internal/metrics"
	"github.com/ServiceWeaver/weaver/internal/proxy"
	"github.com/ServiceWeaver/weaver/internal/routing"
	"github.com/ServiceWeaver/weaver/internal/status"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/envelope"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/perfetto"
	"github.com/ServiceWeaver/weaver/runtime/profiling"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The default number of times a component is replicated.
const defaultReplication = 2

// A deployer manages an application deployment.
type deployer struct {
	ctx          context.Context
	ctxCancel    context.CancelFunc
	deploymentId string
	config       *protos.AppConfig
	started      time.Time
	logger       *slog.Logger
	running      errgroup.Group
	logsDB       *logging.FileStore
	traceDB      *perfetto.DB

	// statsProcessor tracks and computes stats to be rendered on the /statusz page.
	statsProcessor *imetrics.StatsProcessor

	// The deployer places components into co-location groups based on the
	// colocate stanza in a config. For example, consider the following config.
	//
	//     colocate = [
	//         ["A", "B", "C"],
	//         ["D", "E"],
	//     ]
	//
	// The deployer creates a co-location group with components "A", "B", and
	// "C" and a co-location group with components "D" and "E". All other
	// components are placed in their own co-location group. We use the first
	// listed component as the co-location group name.
	//
	// colocation maps components listed in the colocate stanza to the name of
	// their group.
	colocation map[string]string

	mu      sync.Mutex            // guards the following
	err     error                 // error that stopped the babysitter
	groups  map[string]*group     // groups, by group name
	proxies map[string]*proxyInfo // proxies, by listener name

}

// A group contains information about a co-location group.
type group struct {
	name        string                          // group name
	envelopes   []*envelope.Envelope            // envelopes, one per weavelet
	pids        []int64                         // weavelet pids
	components  map[string]bool                 // started components
	addresses   map[string]bool                 // weavelet addresses
	assignments map[string]*protos.Assignment   // assignment, by component
	subscribers map[string][]*envelope.Envelope // routing info subscribers, by component
}

// A proxyInfo contains information about a proxy.
type proxyInfo struct {
	listener string       // listener associated with the proxy
	proxy    *proxy.Proxy // the proxy
	addr     string       // dialable address of the proxy
}

// handler handles a connection to a weavelet.
type handler struct {
	*deployer
	g          *group
	envelope   *envelope.Envelope
	subscribed map[string]bool // routing info subscriptions, by component
}

var _ envelope.EnvelopeHandler = &handler{}

// newDeployer creates a new deployer. The deployer can be stopped at any
// time by canceling the passed-in context.
func newDeployer(ctx context.Context, deploymentId string, config *protos.AppConfig) (*deployer, error) {
	// Create the log saver.
	logsDB, err := logging.NewFileStore(logdir)
	if err != nil {
		return nil, fmt.Errorf("cannot create log storage: %w", err)
	}
	logger := slog.New(&logging.LogHandler{
		Opts: logging.Options{
			App:       config.Name,
			Component: "deployer",
			Weavelet:  uuid.NewString(),
			Attrs:     []string{"serviceweaver/system", ""},
		},
		Write: logsDB.Add,
	})

	// Create the trace saver.
	traceDB, err := perfetto.Open(ctx, "multi")
	if err != nil {
		return nil, fmt.Errorf("cannot open Perfetto database: %w", err)
	}

	// Form co-location.
	colocation := map[string]string{}
	for _, group := range config.Colocate {
		for _, c := range group.Components {
			colocation[c] = group.Components[0]
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	d := &deployer{
		ctx:            ctx,
		ctxCancel:      cancel,
		logger:         logger,
		logsDB:         logsDB,
		traceDB:        traceDB,
		statsProcessor: imetrics.NewStatsProcessor(),
		deploymentId:   deploymentId,
		config:         config,
		started:        time.Now(),
		colocation:     colocation,
		groups:         map[string]*group{},
		proxies:        map[string]*proxyInfo{},
	}

	// Start a goroutine that collects metrics.
	d.running.Go(func() error {
		err := d.statsProcessor.CollectMetrics(d.ctx, d.readMetrics)
		d.stop(err)
		return err
	})

	// Start a goroutine that watches for context cancelation.
	d.running.Go(func() error {
		<-d.ctx.Done()
		err := d.ctx.Err()
		d.stop(err)
		return err
	})

	return d, nil
}

// wait waits for the deployer to terminate. It returns an error that
// caused the deployer to terminate. This method will never return
// a non-nil error.
//
// REQUIRES: d.mu is NOT held.
func (d *deployer) wait() error {
	d.running.Wait() //nolint:errcheck // supplanted by b.err
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.err
}

// REQUIRES: err != nil
// REQUIRES: d.mu is NOT held.
func (d *deployer) stop(err error) {
	// Record the first error.
	d.mu.Lock()
	if d.err == nil {
		d.err = err
	}
	d.mu.Unlock()

	// Cancel the context.
	d.ctxCancel()
}

// group returns the co-location group containing the provided component.
//
// REQUIRES: d.mu is held.
func (d *deployer) group(component string) *group {
	name, ok := d.colocation[component]
	if !ok {
		name = component
	}

	g, ok := d.groups[name]
	if !ok {
		g = &group{
			name:        name,
			components:  map[string]bool{},
			addresses:   map[string]bool{},
			assignments: map[string]*protos.Assignment{},
			subscribers: map[string][]*envelope.Envelope{},
		}
		d.groups[name] = g
	}
	return g
}

// routing returns the RoutingInfo for the provided component.
//
// REQUIRES: d.mu is held.
func (g *group) routing(component string) *protos.RoutingInfo {
	return &protos.RoutingInfo{
		Component:  component,
		Replicas:   maps.Keys(g.addresses),
		Assignment: g.assignments[component],
	}
}

// startColocationGroup starts the colocation group hosting the provided
// component, if it hasn't been started already.
//
// REQUIRES: d.mu is held.
func (d *deployer) startColocationGroup(g *group) error {
	// Check if the deployer has already been stopped. The cleanup protocol
	// requires that no further envelopes be started after the deployer
	// has been stopped.
	if d.err != nil {
		return d.err
	}
	if len(g.envelopes) == defaultReplication {
		// Already started.
		return nil
	}

	components := maps.Keys(g.components)
	for r := 0; r < defaultReplication; r++ {
		// Start the weavelet and capture its logs, traces, and metrics.
		info := &protos.EnvelopeInfo{
			App:           d.config.Name,
			DeploymentId:  d.deploymentId,
			Id:            uuid.New().String(),
			Sections:      d.config.Sections,
			SingleProcess: false,
			SingleMachine: true,
			RunMain:       g.components["github.com/ServiceWeaver/weaver/Main"],
		}
		e, err := envelope.NewEnvelope(d.ctx, info, d.config)
		if err != nil {
			return err
		}

		// Make sure the version of the deployer matches the version of the
		// compiled binary.
		wlet := e.WeaveletInfo()
		if err := checkVersion(wlet.Version); err != nil {
			return err
		}

		d.running.Go(func() error {
			h := &handler{
				deployer:   d,
				g:          g,
				subscribed: map[string]bool{},
				envelope:   e,
			}
			err := e.Serve(h)
			d.stop(err)
			return err
		})
		if err := d.registerReplica(g, wlet); err != nil {
			return err
		}
		if err := e.UpdateComponents(components); err != nil {
			return err
		}
		g.envelopes = append(g.envelopes, e)
	}
	return nil
}

// checkVersion checks that the deployer API version the deployer was built
// with is compatible with the deployer API version the app was built with,
// erroring out if they are not compatible.
func checkVersion(appVersion *protos.SemVer) error {
	if appVersion == nil {
		return fmt.Errorf("version mismatch: nil app version")
	}
	if appVersion.Major != runtime.Major ||
		appVersion.Minor != runtime.Minor ||
		appVersion.Patch != runtime.Patch {
		return fmt.Errorf(
			"version mismatch: deployer version %d.%d.%d is incompatible with app version %d.%d.%d.",
			runtime.Major, runtime.Minor, runtime.Patch,
			appVersion.Major, appVersion.Minor, appVersion.Patch,
		)
	}
	return nil
}

func (d *deployer) startMain() error {
	return d.activateComponent(&protos.ActivateComponentRequest{
		Component: "github.com/ServiceWeaver/weaver/Main",
	})
}

// ActivateComponent implements the envelope.EnvelopeHandler interface.
func (h *handler) ActivateComponent(_ context.Context, req *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error) {
	if err := h.subscribeTo(req); err != nil {
		return nil, err
	}
	return &protos.ActivateComponentReply{}, h.activateComponent(req)
}

func (h *handler) subscribeTo(req *protos.ActivateComponentRequest) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.subscribed[req.Component] {
		return nil
	}
	h.subscribed[req.Component] = true

	target := h.group(req.Component)
	if !req.Routed && h.g.name == target.name {
		// Route locally.
		routing := &protos.RoutingInfo{Component: req.Component, Local: true}
		return h.envelope.UpdateRoutingInfo(routing)
	}

	// Route remotely.
	target.subscribers[req.Component] = append(target.subscribers[req.Component], h.envelope)
	return h.envelope.UpdateRoutingInfo(target.routing(req.Component))
}

func (d *deployer) activateComponent(req *protos.ActivateComponentRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Update the set of components in the target co-location group.
	target := d.group(req.Component)
	if !target.components[req.Component] {
		target.components[req.Component] = true

		// Notify the weavelets.
		components := maps.Keys(target.components)
		for _, envelope := range target.envelopes {
			if err := envelope.UpdateComponents(components); err != nil {
				return err
			}
		}

		// Create an initial assignment.
		if req.Routed {
			replicas := maps.Keys(target.addresses)
			assignment := routingAlgo(&protos.Assignment{}, replicas)
			target.assignments[req.Component] = assignment
			d.logger.Debug(fmt.Sprintf("Initial assignment for component %s:\n%s", req.Component, routing.FormatAssignment(assignment)))
		}

		// Notify the subscribers.
		routing := target.routing(req.Component)
		for _, sub := range target.subscribers[req.Component] {
			if err := sub.UpdateRoutingInfo(routing); err != nil {
				return err
			}
		}
	}

	// Start the co-location group, if it hasn't started already.
	return d.startColocationGroup(target)
}

// registerReplica registers the information about a colocation group replica
// (i.e., a weavelet).
func (d *deployer) registerReplica(g *group, info *protos.WeaveletInfo) error {
	// Update addresses and pids.
	if g.addresses[info.DialAddr] {
		// Replica already registered.
		return nil
	}
	g.addresses[info.DialAddr] = true
	g.pids = append(g.pids, info.Pid)

	// Update all assignments.
	replicas := maps.Keys(g.addresses)
	for component, assignment := range g.assignments {
		assignment = routingAlgo(assignment, replicas)
		g.assignments[component] = assignment
		d.logger.Debug(fmt.Sprintf("Updated assignment for component %s:\n%s", component, routing.FormatAssignment(assignment)))
	}

	// Notify subscribers.
	for component := range g.components {
		routing := g.routing(component)
		for _, sub := range g.subscribers[component] {
			if err := sub.UpdateRoutingInfo(routing); err != nil {
				return err
			}
		}
	}

	return nil
}

// HandleLogEntry implements the envelope.EnvelopeHandler interface.
func (d *deployer) HandleLogEntry(_ context.Context, entry *protos.LogEntry) error {
	d.logsDB.Add(entry)
	return nil
}

// HandleTraceSpans implements the envelope.EnvelopeHandler interface.
func (d *deployer) HandleTraceSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	return d.traceDB.Store(ctx, d.config.Name, d.deploymentId, spans)
}

// GetListenerAddress implements the envelope.EnvelopeHandler interface.
func (d *deployer) GetListenerAddress(context.Context, *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
	return &protos.GetListenerAddressReply{Address: "localhost:0"}, nil
}

// ExportListener implements the envelope.EnvelopeHandler interface.
func (d *deployer) ExportListener(_ context.Context, req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Update the proxy.
	if p, ok := d.proxies[req.Listener]; ok {
		p.proxy.AddBackend(req.Address)
		return &protos.ExportListenerReply{ProxyAddress: p.addr}, nil
	}

	lis, err := net.Listen("tcp", req.LocalAddress)
	if errors.Is(err, syscall.EADDRINUSE) {
		// Don't retry if this address is already in use.
		return &protos.ExportListenerReply{Error: err.Error()}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("proxy listen: %w", err)
	}
	addr := lis.Addr().String()
	d.logger.Info("Proxy listening", "address", addr)
	proxy := proxy.NewProxy(d.logger)
	proxy.AddBackend(req.Address)
	d.proxies[req.Listener] = &proxyInfo{
		listener: req.Listener,
		proxy:    proxy,
		addr:     addr,
	}
	go func() {
		if err := serveHTTP(d.ctx, lis, proxy); err != nil {
			d.logger.Error("proxy", "err", err)
		}
	}()
	return &protos.ExportListenerReply{ProxyAddress: addr}, nil
}

// VerifyClientCertificate implements the envelope.EnvelopeHandler interface.
func (d *deployer) VerifyClientCertificate(context.Context, *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error) {
	// TODO(spetrovic): Implement this functionality.
	panic("unimplemented")
}

// VerifyServerCertificate implements the envelope.EnvelopeHandler interface.
func (d *deployer) VerifyServerCertificate(context.Context, *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error) {
	// TODO(spetrovic): Implement this functionality.
	panic("unimplemented")
}

func (d *deployer) readMetrics() []*metrics.MetricSnapshot {
	d.mu.Lock()
	defer d.mu.Unlock()

	var ms []*metrics.MetricSnapshot
	for _, group := range d.groups {
		for _, envelope := range group.envelopes {
			m, err := envelope.GetMetrics()
			if err != nil {
				continue
			}
			ms = append(ms, m...)
		}
	}
	return append(ms, metrics.Snapshot()...)
}

// Profile implements the status.Server interface.
func (d *deployer) Profile(_ context.Context, req *protos.GetProfileRequest) (*protos.GetProfileReply, error) {
	// Make a copy of the envelopes, so we can operate on it without holding the
	// lock. A profile can last a long time.
	d.mu.Lock()
	envelopes := map[string][]*envelope.Envelope{}
	for _, group := range d.groups {
		envelopes[group.name] = slices.Clone(group.envelopes)
	}
	d.mu.Unlock()

	profile, err := runProfiling(d.ctx, req, envelopes)
	if err != nil {
		return nil, err
	}
	return profile, nil
}

// Status implements the status.Server interface.
func (d *deployer) Status(context.Context) (*status.Status, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	stats := d.statsProcessor.GetStatsStatusz()
	var components []*status.Component
	for _, group := range d.groups {
		for component := range group.components {
			c := &status.Component{
				Name:  component,
				Group: group.name,
				Pids:  slices.Clone(group.pids),
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

	var listeners []*status.Listener
	for _, proxy := range d.proxies {
		listeners = append(listeners, &status.Listener{
			Name: proxy.listener,
			Addr: proxy.addr,
		})
	}

	return &status.Status{
		App:            d.config.Name,
		DeploymentId:   d.deploymentId,
		SubmissionTime: timestamppb.New(d.started),
		Components:     components,
		Listeners:      listeners,
		Config:         d.config,
	}, nil
}

// Metrics implements the status.Server interface.
func (d *deployer) Metrics(context.Context) (*status.Metrics, error) {
	m := &status.Metrics{}
	for _, snap := range d.readMetrics() {
		m.Metrics = append(m.Metrics, snap.ToProto())
	}
	return m, nil
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

// runProfiling runs a profiling request on a set of processes.
func runProfiling(_ context.Context, req *protos.GetProfileRequest, processes map[string][]*envelope.Envelope) (*protos.GetProfileReply, error) {
	// Collect together the groups we want to profile.
	groups := make([][]func() ([]byte, error), 0, len(processes))
	for _, envelopes := range processes {
		group := make([]func() ([]byte, error), 0, len(envelopes))
		for _, e := range envelopes {
			group = append(group, func() ([]byte, error) {
				return e.GetProfile(req)
			})
		}
		groups = append(groups, group)
	}
	data, err := profiling.ProfileGroups(groups)
	return &protos.GetProfileReply{Data: data}, err
}
