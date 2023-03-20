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
	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/logtype"
	"github.com/ServiceWeaver/weaver/internal/versioned"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/envelope"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
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
	ctx    context.Context      // cancels all running envelopes
	t      testing.TB           // the unit test
	wlet   *protos.WeaveletInfo // info for subprocesses
	config *protos.AppConfig    // application config
	logger logtype.Logger       // logger

	mu      sync.Mutex        // guards groups
	groups  map[string]*group // groups, by group name
	stopped sync.WaitGroup    // waits for envelopes to stop

	logMu sync.Mutex // guards log
	log   bool       // logging enabled?
}

var _ envelope.EnvelopeHandler = &deployer{}

// A group contains information about a co-location group.
type group struct {
	name       string                                    // group name
	envelopes  []*envelope.Envelope                      // envelopes, one per weavelet
	components *versioned.Versioned[map[string]bool]     // started components
	routing    *versioned.Versioned[*protos.RoutingInfo] // routing info
}

// newDeployer returns a new weavertest multiprocess deployer.
func newDeployer(ctx context.Context, t testing.TB, wlet *protos.WeaveletInfo, config *protos.AppConfig) *deployer {
	ctx, cancel := context.WithCancel(ctx)
	d := &deployer{
		ctx:    ctx,
		t:      t,
		wlet:   wlet,
		config: config,
		groups: map[string]*group{},
		log:    true,
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
	wlet := &protos.WeaveletInfo{
		App:           d.wlet.App,
		DeploymentId:  d.wlet.DeploymentId,
		Group:         &protos.ColocationGroup{Name: "main"},
		GroupId:       uuid.New().String(),
		Id:            uuid.New().String(),
		SameProcess:   d.wlet.SameProcess,
		Sections:      d.wlet.Sections,
		SingleProcess: d.wlet.SingleProcess,
		SingleMachine: d.wlet.SingleMachine,
	}
	conn, err := conn.NewEnvelopeConn(fromWeaveletReader, toWeaveletWriter, d, wlet)
	if err != nil {
		d.t.Fatalf("cannot create envelope conn: %v", err)
	}
	go func() {
		// TODO(mwhittaker): Close this conn when the unit test ends. Right
		// now, the conn lives forever. This means the pipes are also leaking.
		// We might have to add a Close method to EnvelopeConn.
		if err := conn.Run(); err != nil {
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

// RegisterReplica implements the envelope.EnvelopeHandler interface.
func (d *deployer) RegisterReplica(req *protos.ReplicaToRegister) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	group := d.group(req.Group)
	group.routing.Lock()
	defer group.routing.Unlock()
	for _, replica := range group.routing.Val.Replicas {
		if req.Address == replica {
			// Replica already registered.
			return nil
		}
	}
	group.routing.Val.Replicas = append(group.routing.Val.Replicas, req.Address)
	return nil
}

// StartComponent implements the envelope.EnvelopeHandler interface.
func (d *deployer) StartComponent(req *protos.ComponentToStart) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	group := d.group(req.ColocationGroup)
	group.components.Lock()
	defer group.components.Unlock()
	if group.components.Val[req.Component] {
		// Component already started.
		return nil
	}
	group.components.Val[req.Component] = true
	return d.startGroup(group)
}

// startGroup starts the provided co-location group in a subprocess, if it
// hasn't already been started.
//
// REQUIRES: d.mu is held.
func (d *deployer) startGroup(group *group) error {
	for r := 0; r < DefaultReplication; r++ {
		// Start the weavelet.
		wlet := &protos.WeaveletInfo{
			App:           d.wlet.App,
			DeploymentId:  d.wlet.DeploymentId,
			Group:         &protos.ColocationGroup{Name: group.name},
			GroupId:       uuid.New().String(),
			Id:            uuid.New().String(),
			SameProcess:   d.wlet.SameProcess,
			Sections:      d.wlet.Sections,
			SingleProcess: d.wlet.SingleProcess,
			SingleMachine: d.wlet.SingleMachine,
		}
		e, err := envelope.NewEnvelope(wlet, d.config, d)
		if err != nil {
			return err
		}
		group.envelopes = append(group.envelopes, e)

		// Run the envelope.
		//
		// TODO(mwhittaker): We should add 'd.stopped.Add(1)' and 'defer
		// d.stopped.Done()' calls here, but for some reason, e.Run() is not
		// terminating, even after we successfully call e.Stop.
		go func() {
			// TODO(mwhittaker): If e.Run fails because we called Stop, that's
			// expected. If e.Run fails for any other reason, we should call
			// d.t.Error.
			if err := e.Run(d.ctx); err != nil {
				d.logger.Error("e.Run", err)
			}
		}()
	}
	return nil
}

// GetComponentsToStart implements the envelope.EnvelopeHandler interface.
func (d *deployer) GetComponentsToStart(req *protos.GetComponentsToStart) (*protos.ComponentsToStart, error) {
	d.mu.Lock()
	group := d.group(req.Group)
	d.mu.Unlock()

	// RLock blocks, so we can't hold the lock.
	version := group.components.RLock(req.Version)
	defer group.components.RUnlock()
	return &protos.ComponentsToStart{
		Version:    version,
		Components: maps.Keys(group.components.Val),
	}, nil
}

// GetRoutingInfo implements the envelope.EnvelopeHandler interface.
func (d *deployer) GetRoutingInfo(req *protos.GetRoutingInfo) (*protos.RoutingInfo, error) {
	d.mu.Lock()
	group := d.group(req.Group)
	d.mu.Unlock()

	// RLock blocks, so we can't hold the lock.
	version := group.routing.RLock(req.Version)
	defer group.routing.RUnlock()
	routing := protomsg.Clone(group.routing.Val)
	routing.Version = version
	return routing, nil
}

// group returns the named co-location group.
//
// REQUIRES: d.mu is held.
func (d *deployer) group(name string) *group {
	g, ok := d.groups[name]
	if !ok {
		g = &group{
			name:       name,
			components: versioned.Version(map[string]bool{}),
			routing:    versioned.Version(&protos.RoutingInfo{}),
		}
		d.groups[name] = g
	}
	return g
}
