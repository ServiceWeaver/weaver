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
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/logtype"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/envelope"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/exp/maps"
)

// The default number of times a component is replicated.
//
// TODO(mwhittaker): Include this in the Options struct?
const DefaultReplication = 2

// TODO(mwhittaker): Upgrade to go 1.20 and replace with errors.Join.
type errlist struct {
	errs []error
}

func (e errlist) Error() string {
	var b strings.Builder
	for _, err := range e.errs {
		fmt.Fprintln(&b, err.Error())
	}
	return b.String()
}

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
	ctx        context.Context           // cancels all running envelopes
	t          testing.TB                // the unit test
	wlet       *protos.WeaveletSetupInfo // info for subprocesses
	config     *protos.AppConfig         // application config
	logger     logtype.Logger            // logger
	colocation map[string]string         // maps component to group
	stopped    sync.WaitGroup            // waits for envelopes to stop

	mu     sync.Mutex        // guards groups
	groups map[string]*group // groups, by group name

	logMu sync.Mutex // guards log
	log   bool       // logging enabled?
}

// A group contains information about a co-location group.
type group struct {
	name        string                  // group name
	envelopes   []*envelope.Envelope    // envelopes, one per weavelet
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
	envelope *envelope.Envelope     // envelope to non-main weavelet
	conn     *envelope.EnvelopeConn // conn to main weavelet
}

var _ envelope.EnvelopeHandler = &handler{}

// newDeployer returns a new weavertest multiprocess deployer.
func newDeployer(ctx context.Context, t testing.TB, wlet *protos.WeaveletSetupInfo, config *protos.AppConfig) *deployer {
	colocation := map[string]string{}
	for _, group := range config.SameProcess {
		for _, c := range group.Components {
			colocation[c] = group.Components[0]
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	d := &deployer{
		ctx:        ctx,
		t:          t,
		wlet:       wlet,
		config:     config,
		colocation: colocation,
		groups:     map[string]*group{},
		log:        true,
	}
	d.logger = logging.FuncLogger{
		Opts: logging.Options{
			App:       wlet.App,
			Component: "deployer",
			Weavelet:  uuid.NewString(),
			Attrs:     []string{"serviceweaver/system", ""},
		},
		Write: d.RecvLogEntry,
	}

	t.Cleanup(func() {
		// When the unit test ends, kill all subprocs and stop all goroutines.
		cancel()
		if err := d.Stop(); err != nil {
			d.logger.Error("Stop", err)
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
	wlet := &protos.WeaveletSetupInfo{
		App:           d.wlet.App,
		DeploymentId:  d.wlet.DeploymentId,
		Id:            uuid.New().String(),
		Sections:      d.wlet.Sections,
		SingleProcess: d.wlet.SingleProcess,
		SingleMachine: d.wlet.SingleMachine,
		RunMain:       true,
	}
	// TODO(mwhittaker): Issue an UpdateComponents call to the main group.
	// Right now, the main co-location group knows to start main, but in the
	// future, it won't.
	go func() {
		// NOTE: We must create the envelope conn in a separate gorotuine
		// because it initiates a blocking handshake with the weavelet
		// (initialized below via weaver.Init).
		g := d.group("main")
		handler := &handler{
			deployer:   d,
			group:      g,
			subscribed: map[string]bool{},
		}
		conn, err := envelope.NewEnvelopeConn(fromWeaveletReader, toWeaveletWriter, handler, wlet)
		if err != nil {
			panic(fmt.Errorf("cannot create envelope conn: %w", err))
		}
		handler.conn = connection{conn: conn}
		if err := d.registerReplica(g, conn.WeaveletInfo()); err != nil {
			panic(fmt.Errorf(`cannot register the replica for "main"`))
		}

		// TODO(mwhittaker): Close this conn when the unit test ends. Right
		// now, the conn lives forever. This means the pipes are also leaking.
		// We might have to add a Close method to EnvelopeConn.
		if err := conn.Serve(); err != nil {
			d.t.Error(err)
		}
	}()

	bootstrap := runtime.Bootstrap{
		ToWeaveletFile: toWeaveletReader,
		ToEnvelopeFile: fromWeaveletWriter,
		TestConfig:     config,
	}
	ctx := context.WithValue(d.ctx, runtime.BootstrapKey{}, bootstrap)
	return weaver.Init(ctx)
}

// Stop shuts down the deployment. It blocks until the deployment is fully
// destroyed.
func (d *deployer) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Stop envelopes.
	var errs []error
	for _, group := range d.groups {
		for _, envelope := range group.envelopes {
			if err := envelope.Stop(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	// Wait for all goroutines to end.
	d.stopped.Wait()

	if len(errs) > 0 {
		return errlist{errs}
	}
	return nil
}

// RecvLogEntry implements the envelope.EnvelopeHandler interface.
func (d *deployer) RecvLogEntry(entry *protos.LogEntry) {
	d.logMu.Lock()
	defer d.logMu.Unlock()
	if d.log {
		// NOTE(mwhittaker): We intentionally create a new pretty printer for
		// every log entry. If we used a single pretty printer, it would
		// perform dimming, but when the dimmed output is interspersed with
		// various other test logs, it is confusing.
		d.t.Log(logging.NewPrettyPrinter(colors.Enabled()).Format(entry))
	}
}

// RecvTraceSpans implements the envelope.EnvelopeHandler interface.
func (d *deployer) RecvTraceSpans([]trace.ReadOnlySpan) error {
	// Ignore traces.
	return nil
}

// ReportLoad implements the envelope.EnvelopeHandler interface.
func (d *deployer) ReportLoad(*protos.WeaveletLoadReport) error {
	// Ignore load.
	return nil
}

// GetAddress implements the envelope.EnvelopeHandler interface.
func (d *deployer) GetAddress(req *protos.GetAddressRequest) (*protos.GetAddressReply, error) {
	return &protos.GetAddressReply{Address: "localhost:0"}, nil
}

// ExportListener implements the envelope.EnvelopeHandler interface.
func (d *deployer) ExportListener(req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
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

// StartComponent implements the envelope.EnvelopeHandler interface.
func (h *handler) StartComponent(req *protos.ComponentToStart) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Update the set of components in the target co-location group.
	target := h.deployer.group(req.Component)
	if !target.components[req.Component] {
		target.components[req.Component] = true

		// Notify the weavelets.
		components := maps.Keys(target.components)
		for _, envelope := range target.envelopes {
			if err := envelope.UpdateComponents(components); err != nil {
				return err
			}
		}

		// Notify the subscribers.
		routing := target.routing(req.Component)
		for _, sub := range target.subscribers[req.Component] {
			if err := sub.UpdateRoutingInfo(routing); err != nil {
				return err
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
				return err
			}
		} else {
			// Route remotely.
			target.subscribers[req.Component] = append(target.subscribers[req.Component], h.conn)
			if err := h.conn.UpdateRoutingInfo(target.routing(req.Component)); err != nil {
				return err
			}
		}
	}

	// Start the co-location group
	return h.deployer.startGroup(target)
}

// startGroup starts the provided co-location group in a subprocess, if it
// hasn't already been started.
//
// REQUIRES: d.mu is held.
func (d *deployer) startGroup(g *group) error {
	if len(g.envelopes) > 0 {
		// Envelopes already started
		return nil
	}

	components := maps.Keys(g.components)
	for r := 0; r < DefaultReplication; r++ {
		// Start the weavelet.
		wlet := &protos.WeaveletSetupInfo{
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
		e, err := envelope.NewEnvelope(d.ctx, wlet, d.config, handler)
		if err != nil {
			return err
		}
		handler.conn = connection{envelope: e}

		// Run the envelope.
		//
		// TODO(mwhittaker): We should add 'd.stopped.Add(1)' and 'defer
		// d.stopped.Done()' calls here, but for some reason, e.Serve() is not
		// terminating, even after we successfully call e.Stop.
		go func() {
			// TODO(mwhittaker): If e.Serve fails because we called Stop, that's
			// expected. If e.Serve fails for any other reason, we should call
			// d.t.Error.
			if err := e.Serve(d.ctx); err != nil {
				d.logger.Error("e.Run", err)
			}
		}()
		if err := d.registerReplica(g, e.WeaveletInfo()); err != nil {
			return err
		}
		if err := e.UpdateComponents(components); err != nil {
			return err
		}
		g.envelopes = append(g.envelopes, e)
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
	return c.envelope.UpdateRoutingInfo(routing)
}

// UpdateComponents is equivalent to Envelope.UpdateComponents.
func (c connection) UpdateComponents(components []string) error {
	return c.envelope.UpdateComponents(components)
}
