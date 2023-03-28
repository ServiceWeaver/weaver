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

package weavertest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/envelope"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
)

// The default number of times a component is replicated.
//
// TODO(mwhittaker): Include this in the Options struct?
const DefaultReplication = 2

// deployer is the weavertest multiprocess deployer. Every multiprocess
// weavertest runs its own deployer. The main component is run in the same
// process as the deployer, which is the same process as the unit test. All
// other components are run in subprocesses.
//
// This deployer differs from 'weaver multi' in two key ways.
//
//  1. This deployer doesn't implement unneeded features (e.g., traces,
//     metrics, routing, health checking). This greatly simplifies the
//     implementation.
//  2. This deployer handles the fact that the main component is run in the
//     same process as the deployer. This is special to weavertests and
//     requires special care. See Init for more details.
type deployer struct {
	ctx        context.Context
	ctxCancel  context.CancelFunc
	t          testing.TB           // the unit test
	wlet       *protos.EnvelopeInfo // info for subprocesses
	config     *protos.AppConfig    // application config
	logger     *slog.Logger         // logger
	colocation map[string]string    // maps component to group
	running    errgroup.Group

	logMu sync.Mutex // guards log
	log   bool       // logging enabled?

	mu     sync.Mutex        // guards fields below
	groups map[string]*group // groups, by group name
	err    error             // error the test was terminated with, if any.
}

// A group contains information about a co-location group.
type group struct {
	name        string                  // group name
	conns       []connection            // connections to the weavelet
	components  map[string]bool         // started components
	addresses   map[string]bool         // weavelet addresses
	subscribers map[string][]connection // routing info subscribers, by component
}

// handler handles a connection to a weavelet.
type handler struct {
	*deployer
	group      *group
	conn       connection
	subscribed map[string]bool // routing info subscriptions, by component
}

// A connection is either an Envelope or an EnvelopeConn.
//
// The weavertest deployer has to deal with the annoying fact that the main
// weavelet runs in the same process as the deployer. Thus, the connection to
// the main weavelet is an EnvelopeConn. On the other hand, the connection to
// all other weavelets is an Envelope. A connection encapsulates the common
// logic between the two.
//
// TODO(mwhittaker): Can we revise the Envelope API to avoid having to do stuff
// like this? Having a weavelet run in the same process is rare though, so it
// might not be worth it.
type connection struct {
	// One of the following fields will be nil.
	envelope *envelope.Envelope // envelope to non-main weavelet
	conn     *conn.EnvelopeConn // conn to main weavelet
}

var _ envelope.EnvelopeHandler = &handler{}

// newDeployer returns a new weavertest multiprocess deployer.
func newDeployer(ctx context.Context, t testing.TB, wlet *protos.EnvelopeInfo, config *protos.AppConfig) *deployer {
	colocation := map[string]string{}
	for _, group := range config.Colocate {
		for _, c := range group.Components {
			colocation[c] = group.Components[0]
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	d := &deployer{
		ctx:        ctx,
		ctxCancel:  cancel,
		t:          t,
		wlet:       wlet,
		config:     config,
		colocation: colocation,
		groups:     map[string]*group{},
		log:        true,
	}
	d.logger = slog.New(&logging.LogHandler{
		Opts: logging.Options{
			App:       wlet.App,
			Component: "deployer",
			Weavelet:  uuid.NewString(),
			Attrs:     []string{"serviceweaver/system", ""},
		},
		Write: func(e *protos.LogEntry) {
			d.HandleLogEntry(d.ctx, e) //nolint:errcheck // TODO(mwhittaker): Propagate error.
		},
	})

	t.Cleanup(func() {
		if err := d.cleanup(); err != nil {
			d.logger.Error("cleanup", err)
		}
		maybeLogStacks()

		// NOTE(mwhittaker): We cannot replace d.logMu with d.mu because we
		// sometimes log with d.mu held. This would deadlock.
		d.logMu.Lock()
		d.log = false
		d.logMu.Unlock()
	})
	return d
}

// Init acts like weaver.Init when called from the main component.
func (d *deployer) Init(config string) weaver.Instance {
	// Set up the pipes between the envelope and the main weavelet. The
	// pipes will be closed by the envelope and weavelet conns.
	//
	//         envelope                      weavelet
	//         ────────        ┌────┐        ────────
	//   fromWeaveletReader <──│ OS │<── fromWeaveletWriter
	//     toWeaveletWriter ──>│    │──> toWeaveletReader
	//                         └────┘
	fromWeaveletReader, fromWeaveletWriter, err := os.Pipe()
	if err != nil {
		d.t.Fatalf("cannot create fromWeavelet pipe: %v", err)
	}
	toWeaveletReader, toWeaveletWriter, err := os.Pipe()
	if err != nil {
		d.t.Fatalf("cannot create toWeavelet pipe: %v", err)
	}
	// Run an envelope connection to the main co-location group.
	wlet := &protos.EnvelopeInfo{
		App:           d.wlet.App,
		DeploymentId:  d.wlet.DeploymentId,
		Id:            uuid.New().String(),
		Sections:      d.wlet.Sections,
		SingleProcess: d.wlet.SingleProcess,
		SingleMachine: d.wlet.SingleMachine,
		RunMain:       true,
	}
	var e *conn.EnvelopeConn
	created := make(chan struct{})
	go func() {
		// NOTE: We must create the envelope conn in a separate gorotuine
		// because it initiates a blocking handshake with the weavelet
		// (initialized below via weaver.Init).
		var err error
		if e, err = conn.NewEnvelopeConn(
			d.ctx, fromWeaveletReader, toWeaveletWriter, wlet); err != nil {
			panic(fmt.Errorf("cannot create envelope conn: %w", err))
		}
		created <- struct{}{}
	}()
	bootstrap := runtime.Bootstrap{
		ToWeaveletFile: toWeaveletReader,
		ToEnvelopeFile: fromWeaveletWriter,
		TestConfig:     config,
	}
	ctx := context.WithValue(d.ctx, runtime.BootstrapKey{}, bootstrap)
	instance := weaver.Init(ctx)
	<-created

	g := d.group("main")
	g.conns = append(g.conns, connection{conn: e})
	handler := &handler{
		deployer:   d,
		group:      g,
		subscribed: map[string]bool{},
		conn:       connection{conn: e},
	}
	d.running.Go(func() error {
		err := e.Serve(handler)
		d.stop(err)
		return err
	})

	if err := d.registerReplica(g, e.WeaveletInfo()); err != nil {
		panic(fmt.Errorf(`cannot register the replica for "main"`))
	}

	return instance
}

func (d *deployer) stop(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err == nil && !errors.Is(err, context.Canceled) {
		d.err = err
	}
	d.ctxCancel()
}

// cleanup cleans up all of the running envelopes' state.
func (d *deployer) cleanup() error {
	d.ctxCancel()
	d.running.Wait() //nolint:errcheck // supplanted by b.err
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.err
}

// HandleLogEntry implements the envelope.EnvelopeHandler interface.
func (d *deployer) HandleLogEntry(_ context.Context, entry *protos.LogEntry) error {
	d.logMu.Lock()
	defer d.logMu.Unlock()
	if d.log {
		// NOTE(mwhittaker): We intentionally create a new pretty printer for
		// every log entry. If we used a single pretty printer, it would
		// perform dimming, but when the dimmed output is interspersed with
		// various other test logs, it is confusing.
		d.t.Log(logging.NewPrettyPrinter(colors.Enabled()).Format(entry))
	}
	return nil
}

// HandleTraceSpans implements the envelope.EnvelopeHandler interface.
func (d *deployer) HandleTraceSpans(context.Context, []trace.ReadOnlySpan) error {
	// Ignore traces.
	return nil
}

// GetListenerAddress implements the envelope.EnvelopeHandler interface.
func (d *deployer) GetListenerAddress(_ context.Context, req *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
	return &protos.GetListenerAddressReply{Address: "localhost:0"}, nil
}

// ExportListener implements the envelope.EnvelopeHandler interface.
func (d *deployer) ExportListener(_ context.Context, req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	return &protos.ExportListenerReply{}, nil
}

// registerReplica registers the information about a colocation group replica
// (i.e., a weavelet).
func (d *deployer) registerReplica(g *group, info *protos.WeaveletInfo) error {
	// Update addresses.
	if g.addresses[info.DialAddr] {
		// Replica already registered.
		return nil
	}
	g.addresses[info.DialAddr] = true

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

// ActivateComponent implements the envelope.EnvelopeHandler interface.
func (h *handler) ActivateComponent(_ context.Context, req *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Update the set of components in the target co-location group.
	target := h.deployer.group(req.Component)
	if !target.components[req.Component] {
		target.components[req.Component] = true

		// Notify the weavelets.
		components := maps.Keys(target.components)
		for _, envelope := range target.conns {
			if err := envelope.UpdateComponents(components); err != nil {
				return nil, err
			}
		}

		// Notify the subscribers.
		routing := target.routing(req.Component)
		for _, sub := range target.subscribers[req.Component] {
			if err := sub.UpdateRoutingInfo(routing); err != nil {
				return nil, err
			}
		}
	}

	// Subscribe to the component's routing info.
	if !h.subscribed[req.Component] {
		h.subscribed[req.Component] = true

		if h.group.name == target.name {
			// Route locally.
			routing := &protos.RoutingInfo{Component: req.Component, Local: true}
			if err := h.conn.UpdateRoutingInfo(routing); err != nil {
				return nil, err
			}
		} else {
			// Route remotely.
			target.subscribers[req.Component] = append(target.subscribers[req.Component], h.conn)
			if err := h.conn.UpdateRoutingInfo(target.routing(req.Component)); err != nil {
				return nil, err
			}
		}
	}

	// Start the co-location group
	return &protos.ActivateComponentReply{}, h.deployer.startGroup(target)
}

// startGroup starts the provided co-location group in a subprocess, if it
// hasn't already been started.
//
// REQUIRES: d.mu is held.
func (d *deployer) startGroup(g *group) error {
	if len(g.conns) > 0 {
		// Envelopes already started
		return nil
	}

	components := maps.Keys(g.components)
	for r := 0; r < DefaultReplication; r++ {
		// Start the weavelet.
		wlet := &protos.EnvelopeInfo{
			App:           d.wlet.App,
			DeploymentId:  d.wlet.DeploymentId,
			Id:            uuid.New().String(),
			Sections:      d.wlet.Sections,
			SingleProcess: d.wlet.SingleProcess,
			SingleMachine: d.wlet.SingleMachine,
		}
		handler := &handler{
			deployer:   d,
			group:      g,
			subscribed: map[string]bool{},
		}
		e, err := envelope.NewEnvelope(d.ctx, wlet, d.config)
		if err != nil {
			return err
		}
		d.running.Go(func() error {
			err := e.Serve(handler)
			d.stop(err)
			return err
		})
		if err := d.registerReplica(g, e.WeaveletInfo()); err != nil {
			return err
		}
		if err := e.UpdateComponents(components); err != nil {
			return err
		}
		c := connection{envelope: e}
		handler.conn = c
		g.conns = append(g.conns, c)
	}
	return nil
}

// group returns the group that corresponds to the given component.
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
			subscribers: map[string][]connection{},
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
		Component: component,
		Replicas:  maps.Keys(g.addresses),
	}
}

// UpdateRoutingInfo is equivalent to Envelope.UpdateRoutingInfo.
func (c connection) UpdateRoutingInfo(routing *protos.RoutingInfo) error {
	if c.envelope != nil {
		return c.envelope.UpdateRoutingInfo(routing)
	}
	if c.conn != nil {
		return c.conn.UpdateRoutingInfoRPC(routing)
	}
	panic(fmt.Errorf("nil connection"))
}

// UpdateComponents is equivalent to Envelope.UpdateComponents.
func (c connection) UpdateComponents(components []string) error {
	if c.envelope != nil {
		return c.envelope.UpdateComponents(components)
	}
	if c.conn != nil {
		return c.conn.UpdateComponentsRPC(components)
	}
	panic(fmt.Errorf("nil connection"))
}
