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
	"reflect"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/envelope"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
	"golang.org/x/exp/maps"
	"golang.org/x/sync/errgroup"
)

// DefaultReplication is the default number of times a component is replicated.
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
//     requires special care. See start() for more details.
type deployer struct {
	ctx        context.Context
	ctxCancel  context.CancelFunc
	runner     Runner                 // holds runner-specific info like config
	wlet       *protos.EnvelopeInfo   // info for subprocesses
	config     *protos.AppConfig      // application config
	colocation map[string]string      // maps component to group
	running    errgroup.Group         // collects errors from goroutines
	local      map[string]bool        // Components that should run locally
	log        func(*protos.LogEntry) // logs the passed in string

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

// newDeployer returns a new weavertest multiprocess deployer. locals contains
// components that should be co-located with the main component and not
// replicated.
func newDeployer(ctx context.Context, wlet *protos.EnvelopeInfo, config *protos.AppConfig, runner Runner, locals []reflect.Type, logWriter func(*protos.LogEntry)) *deployer {
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
		runner:     runner,
		wlet:       wlet,
		config:     config,
		colocation: colocation,
		groups:     map[string]*group{},
		local:      map[string]bool{},
		log:        logWriter,
	}

	for _, local := range locals {
		name := fmt.Sprintf("%s/%s", local.PkgPath(), local.Name())
		d.local[name] = true
	}

	// Fakes need to be local as well.
	for _, fake := range runner.Fakes {
		name := fmt.Sprintf("%s/%s", fake.intf.PkgPath(), fake.intf.Name())
		d.local[name] = true
	}

	return d
}

func (d *deployer) start() (runtime.Bootstrap, error) {
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
		return runtime.Bootstrap{}, fmt.Errorf("cannot create fromWeavelet pipe: %v", err)
	}
	toWeaveletReader, toWeaveletWriter, err := os.Pipe()
	if err != nil {
		return runtime.Bootstrap{}, fmt.Errorf("cannot create toWeavelet pipe: %v", err)
	}
	// Run an envelope connection to the main co-location group.
	wlet := &protos.EnvelopeInfo{
		App:             d.wlet.App,
		DeploymentId:    d.wlet.DeploymentId,
		Id:              uuid.New().String(),
		Sections:        d.wlet.Sections,
		InternalAddress: "localhost:0",
	}
	bootstrap := runtime.Bootstrap{
		ToWeaveletFile: toWeaveletReader,
		ToEnvelopeFile: fromWeaveletWriter,
	}
	d.ctx = context.WithValue(d.ctx, runtime.BootstrapKey{}, bootstrap)

	// NOTE: NewEnvelopeConn initiates a blocking handshake with the weavelet
	// and therefore we run the rest of the initialization in a goroutine which
	// will wait for weaver.Run to create a weavelet.
	go func() {
		e, err := conn.NewEnvelopeConn(d.ctx, fromWeaveletReader, toWeaveletWriter, wlet)
		if err != nil {
			d.stop(err)
			return
		}
		d.mu.Lock()
		defer d.mu.Unlock()
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
			d.stopLocked(fmt.Errorf(`cannot register the replica for "main": %w`, err))
		}
	}()
	return bootstrap, nil
}

// stop stops the deployer.
// REQUIRES: d.mu is not held.
func (d *deployer) stop(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopLocked(err)
}

// stopLocked stops the deployer.
// REQUIRES: d.mu is held.
func (d *deployer) stopLocked(err error) {
	if d.err == nil && !errors.Is(err, context.Canceled) {
		d.err = err
	}
	d.ctxCancel()
}

// cleanup cleans up all of the running envelopes' state.
func (d *deployer) cleanup() error {
	d.ctxCancel()
	d.running.Wait()
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.err
}

// HandleLogEntry implements the envelope.EnvelopeHandler interface.
func (d *deployer) HandleLogEntry(_ context.Context, entry *protos.LogEntry) error {
	// NOTE(mwhittaker): We intentionally create a new pretty printer for
	// every log entry. If we used a single pretty printer, it would
	// perform dimming, but when the dimmed output is interspersed with
	// various other test logs, it is confusing.
	d.log(entry)
	return nil
}

// HandleTraceSpans implements the envelope.EnvelopeHandler interface.
func (d *deployer) HandleTraceSpans(context.Context, *protos.TraceSpans) error {
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

func (*deployer) GetSelfCertificate(context.Context, *protos.GetSelfCertificateRequest) (*protos.GetSelfCertificateReply, error) {
	// This deployer doesn't enable mTLS.
	panic("unused")
}

func (*deployer) VerifyClientCertificate(context.Context, *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error) {
	// This deployer doesn't enable mTLS.
	panic("unused")
}

func (*deployer) VerifyServerCertificate(context.Context, *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error) {
	// This deployer doesn't enable mTLS.
	panic("unused")
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

		if !h.runner.forceRPC && h.group.name == target.name {
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
			App:             d.wlet.App,
			DeploymentId:    d.wlet.DeploymentId,
			Id:              uuid.New().String(),
			Sections:        d.wlet.Sections,
			InternalAddress: "localhost:0",
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
	var name string
	if !d.runner.multi {
		name = "main" // Everything is in one group.
	} else if d.local[component] {
		name = "main" // Run locally
	} else if x, ok := d.colocation[component]; ok {
		name = x // Use specified group
	} else {
		name = component // A group of its own
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
