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

// Package run provides a low-level mechanism to run a weavelet when the
// topology of the application (e.g., the address of every listener and
// component) is static and predetermined.
package run

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync"

	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/envelope"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

// Config configures a weavelet run with [Run].
type Config struct {
	ConfigFile      string              // weaver TOML config filename
	DeploymentId    string              // globally unique deployment id
	Components      []string            // components to run
	Resolvers       map[string]Resolver // resolvers for every component in the app
	Listeners       map[string]string   // listener addresses, by listener name
	WeaveletAddress string              // internal weavelet address
}

// Run runs a weavelet with the provided set components.
func Run(ctx context.Context, config Config) error {
	// Validate config.
	if config.ConfigFile == "" {
		return fmt.Errorf("topo.Run: Config.ConfigFile is empty")
	}
	if config.DeploymentId == "" {
		return fmt.Errorf("topo.Run: Config.DeploymentId is empty")
	}
	if len(config.Components) == 0 {
		return fmt.Errorf("topo.Run: Config.Components is empty")
	}
	if config.WeaveletAddress == "" {
		return fmt.Errorf("topo.Run: Config.WeaveletAddress is empty")
	}

	// Read and validate the config file.
	//
	// TODO(mwhittaker): Validate config.Listeners against the listeners stored
	// in the binary.
	bytes, err := os.ReadFile(config.ConfigFile)
	if err != nil {
		return fmt.Errorf("topo.Run: read config file %q: %w\n", config.ConfigFile, err)
	}
	app, err := runtime.ParseConfig(config.ConfigFile, string(bytes), codegen.ComponentConfigValidator)
	if err != nil {
		return fmt.Errorf("topo.Run: parse config file %q: %w\n", config.ConfigFile, err)
	}

	// Spawn a weavelet in a subprocess to host the components.
	info := &protos.EnvelopeInfo{
		App:             app.Name,
		DeploymentId:    config.DeploymentId,
		Id:              uuid.New().String(),
		Sections:        app.Sections,
		RunMain:         slices.Contains(config.Components, runtime.Main),
		InternalAddress: config.WeaveletAddress,
	}
	envelope, err := envelope.NewEnvelope(ctx, info, app)
	if err != nil {
		return fmt.Errorf("topo.Run: create envelope: %w", err)
	}

	// Update the weavelet's components.
	if err := envelope.UpdateComponents(config.Components); err != nil {
		return fmt.Errorf("topo.Run: update components: %w", err)
	}

	// Block serving.
	group, ctx := errgroup.WithContext(ctx)
	h := &handler{
		config:    config,
		envelope:  envelope,
		pp:        logging.NewPrettyPrinter(colors.Enabled()),
		ctx:       ctx,
		resolvers: group,
		activated: map[string]struct{}{},
	}
	group.Go(func() error { return envelope.Serve(h) })
	return group.Wait()
}

// handler is an EnvelopeHandler.
type handler struct {
	config    Config                 // config
	envelope  *envelope.Envelope     // envelope to the weavelet
	pp        *logging.PrettyPrinter // log pretty printer
	ctx       context.Context        // context for resolver goroutines
	resolvers *errgroup.Group        // resolver goroutines, see ActivateComponent

	mu        sync.Mutex          // guards activated
	activated map[string]struct{} // activated commponents
}

var _ envelope.EnvelopeHandler = &handler{}

// HandleLogEntry implements the envelope.EnvelopeHandler interface.
func (h *handler) HandleLogEntry(ctx context.Context, entry *protos.LogEntry) error {
	fmt.Println(h.pp.Format(entry))
	return nil
}

// HandleTraceSpans implements the envelope.EnvelopeHandler interface.
func (h *handler) HandleTraceSpans(context.Context, *protos.TraceSpans) error {
	// TODO(mwhittaker): Implement.
	return nil
}

// ActivateComponent implements the envelope.EnvelopeHandler interface.
func (h *handler) ActivateComponent(ctx context.Context, req *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.activated[req.Component]; ok {
		// This component was already activated.
		return &protos.ActivateComponentReply{}, nil
	}
	h.activated[req.Component] = struct{}{}

	// Check if the component is running locally, in which case all method
	// calls will be routed to the local instance of the component.
	if slices.Contains(h.config.Components, req.Component) {
		routing := &protos.RoutingInfo{Component: req.Component, Local: true}
		if err := h.envelope.UpdateRoutingInfo(routing); err != nil {
			// TODO(mwhittaker): Retry?
			return nil, fmt.Errorf("update routing info for %q: %w", req.Component, err)
		}
		return &protos.ActivateComponentReply{}, nil
	}

	// Spawn a goroutine to watch the resolver and update the weavelet whenever
	// the set addresses changes.
	resolver, ok := h.config.Resolvers[req.Component]
	if !ok {
		return nil, fmt.Errorf("component %q resolver not found", req.Component)
	}
	h.resolvers.Go(func() error {
		for {
			select {
			case <-h.ctx.Done():
				return h.ctx.Err()

			case addrs, ok := <-resolver.Addresses():
				if !ok {
					return nil
				}
				for i, addr := range addrs {
					addrs[i] = "tcp://" + addr
				}
				routing := &protos.RoutingInfo{
					Component: req.Component,
					Replicas:  addrs,
				}
				if err := h.envelope.UpdateRoutingInfo(routing); err != nil {
					// TODO(mwhittaker): Retry?
					return fmt.Errorf("update routing info for %q: %w", req.Component, err)
				}
			}
		}
	})

	return &protos.ActivateComponentReply{}, nil
}

// GetListenerAddress implements the envelope.EnvelopeHandler interface.
func (h *handler) GetListenerAddress(ctx context.Context, lis *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
	addr, ok := h.config.Listeners[lis.Name]
	if !ok {
		return nil, fmt.Errorf("GetListenerAddress: listener %q not found", lis.Name)
	}
	return &protos.GetListenerAddressReply{Address: addr}, nil
}

// ExportListener implements the envelope.EnvelopeHandler interface.
func (h *handler) ExportListener(context.Context, *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	return &protos.ExportListenerReply{}, nil
}

// GetSelfCertificate implements the envelope.EnvelopeHandler interface.
func (h *handler) GetSelfCertificate(context.Context, *protos.GetSelfCertificateRequest) (*protos.GetSelfCertificateReply, error) {
	panic(fmt.Errorf("GetSelfCertificate unimplemented"))
}

// VerifyClientCertificate implements the envelope.EnvelopeHandler interface.
func (h *handler) VerifyClientCertificate(context.Context, *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error) {
	panic(fmt.Errorf("VerifyClientCertificate unimplemented"))
}

// VerifyServerCertificate implements the envelope.EnvelopeHandler interface.
func (h *handler) VerifyServerCertificate(context.Context, *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error) {
	panic(fmt.Errorf("VerifyServerCertificate unimplemented"))
}
