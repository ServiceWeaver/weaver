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

// Package main implements a simple multiprocess deployer. See
// https://serviceweaver.dev/blog/deployers.html for corresponding blog post.
package main

import (
	"context"
	"flag"
	"fmt"
	"sync"

	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/envelope"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
)

// deployer is a simple multiprocess deployer that doesn't implement
// co-location or replication. That is, every component is run in its own OS
// process, and there is only one replica of every component.
type deployer struct {
	mu       sync.Mutex          // guards handlers
	handlers map[string]*handler // handlers, by component
}

// A handler handles messages from a weavelet. It implements the
// EnvelopeHandler interface.
type handler struct {
	deployer *deployer          // underlying deployer
	envelope *envelope.Envelope // envelope to the weavelet
	address  string             // weavelet's address
}

// Check that handler implements the envelope.EnvelopeHandler interface.
var _ envelope.EnvelopeHandler = &handler{}

// The unique id of the application deployment.
var deploymentId = uuid.New().String()

// Usage: ./multi <service weaver binary>
func main() {
	flag.Parse()
	d := &deployer{handlers: map[string]*handler{}}
	d.spawn("main") //nolint:errcheck // omitted for brevity
	select {}       // block forever
}

// spawn spawns a weavelet to host the provided component (if one hasn't
// already spawned) and returns a handler to the weavelet.
func (d *deployer) spawn(component string) (*handler, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if a weavelet has already been spawned.
	h, ok := d.handlers[component]
	if ok {
		// The weavelet has already been spawned.
		return h, nil
	}

	// Spawn a weavelet in a subprocess to host the component.
	info := &protos.EnvelopeInfo{
		App:           "app",               // the application name
		DeploymentId:  deploymentId,        // the deployment id
		Id:            uuid.New().String(), // the weavelet id
		SingleProcess: false,               // is the app a single process?
		SingleMachine: true,                // is the app on a single machine?
		RunMain:       component == "main", // should the weavelet run main?
	}
	config := &protos.AppConfig{
		Name:   "app",       // the application name
		Binary: flag.Arg(0), // the application binary
	}
	envelope, err := envelope.NewEnvelope(context.Background(), info, config)
	if err != nil {
		return nil, err
	}
	h = &handler{
		deployer: d,
		envelope: envelope,
		address:  envelope.WeaveletInfo().DialAddr,
	}

	go func() {
		// Inform the weavelet of the component it should host.
		envelope.UpdateComponents([]string{component}) //nolint:errcheck // omitted for brevity

		// Handle messages from the weavelet.
		envelope.Serve(h) //nolint:errcheck // omitted for brevity
	}()

	// Return the handler.
	d.handlers[component] = h
	return h, nil
}

// Responsibility 1: Components.
func (h *handler) ActivateComponent(_ context.Context, req *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error) {
	// Spawn a weavelet to host the component, if one hasn't already been
	// spawned.
	spawned, err := h.deployer.spawn(req.Component)
	if err != nil {
		return nil, err
	}

	// Tell the weavelet the address of the requested component.
	h.envelope.UpdateRoutingInfo(&protos.RoutingInfo{ //nolint:errcheck // omitted for brevity
		Component: req.Component,
		Replicas:  []string{spawned.address},
	})

	return &protos.ActivateComponentReply{}, nil
}

// Responsibility 2: Listeners.
func (h *handler) GetListenerAddress(_ context.Context, req *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
	return &protos.GetListenerAddressReply{Address: "localhost:0"}, nil
}

func (h *handler) ExportListener(_ context.Context, req *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	// This simplified deployer does not proxy network traffic. Listeners
	// should be contacted directly.
	fmt.Printf("Weavelet listening on %s\n", req.Address)
	return &protos.ExportListenerReply{}, nil
}

func (*handler) GetSelfCertificate(context.Context, *protos.GetSelfCertificateRequest) (*protos.GetSelfCertificateReply, error) {
	panic("unused")
}

func (*handler) VerifyClientCertificate(context.Context, *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error) {
	panic("unused")
}

func (*handler) VerifyServerCertificate(context.Context, *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error) {
	panic("unused")
}

// Responsibility 3: Telemetry.
func (h *handler) HandleLogEntry(_ context.Context, entry *protos.LogEntry) error {
	pp := logging.NewPrettyPrinter(colors.Enabled())
	fmt.Println(pp.Format(entry))
	return nil
}

func (h *handler) HandleTraceSpans(context.Context, *protos.TraceSpans) error {
	// This simplified deployer drops traces on the floor.
	return nil
}
