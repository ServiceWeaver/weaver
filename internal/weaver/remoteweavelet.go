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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/config"
	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/internal/register"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/maps"
	"golang.org/x/sync/errgroup"
)

// readyMethodKey holds the key for a method used to check if a backend is ready.
var readyMethodKey = call.MakeMethodKey("", "ready")

// RemoteWeaveletOptions configure a RemoteWeavelet.
type RemoteWeaveletOptions struct {
	Fakes         map[reflect.Type]any // component fakes, by component interface type
	InjectRetries int                  // Number of artificial retries to inject per retriable call
}

// RemoteWeavelet is a weavelet that runs some components locally, but
// coordinates with a deployer over a set of Unix pipes to start other
// components remotely. It is the weavelet used by all deployers, except for
// the single process deployer.
type RemoteWeavelet struct {
	ctx       context.Context       // shuts down the weavelet when canceled
	servers   *errgroup.Group       // background servers
	opts      RemoteWeaveletOptions // options
	conn      *conn.WeaveletConn    // connection to envelope
	logDst    *remoteLogger         // for writing log entries
	syslogger *slog.Logger          // system logger
	tracer    trace.Tracer          // tracer used by all components

	componentsByName map[string]*component       // component name -> component
	componentsByIntf map[reflect.Type]*component // component interface type -> component
	componentsByImpl map[reflect.Type]*component // component impl type -> component
	redirects        map[string]redirect         // component redirects

	lismu     sync.Mutex           // guards listeners
	listeners map[string]*listener // listeners, by name
}

type redirect struct {
	component *component
	target    string
	address   string
}

// component represents a Service Weaver component and all corresponding
// metadata.
type component struct {
	reg       *codegen.Registration // read-only, once initialized
	clientTLS *tls.Config           // read-only, once initialized

	activateInit sync.Once // used to activate the component
	activateErr  error     // non-nil if activation fails

	implInit   sync.Once      // used to initialize impl, severStub
	implErr    error          // non-nil if impl creation fails
	impl       any            // instance of component implementation
	serverStub codegen.Server // handles remote calls from other processes

	// TODO(mwhittaker): We have one client for every component. Every client
	// independently maintains network connections to every weavelet hosting
	// the component. Thus, there may be many redundant network connections to
	// the same weavelet. Given n weavelets hosting m components, there's at
	// worst n^2m connections rather than a more optimal n^2 (a single
	// connection between every pair of weavelets). We should rewrite things to
	// avoid the redundancy.
	resolver *routingResolver // client resolver
	balancer *routingBalancer // client balancer

	stubInit sync.Once // used to initialize stub
	stubErr  error     // non-nil if stub creation fails
	stub     *stub     // network stub to remote component

	local register.WriteOnce[bool] // routed locally?
	load  *loadCollector           // non-nil for routed components
}

// listener is a network listener and the proxy address that should be used to
// reach that listener.
type listener struct {
	lis       net.Listener
	proxyAddr string
}

// NewRemoteWeavelet returns a new RemoteWeavelet that hosts the components
// specified in the provided registrations. bootstrap is used to establish a
// connection with an envelope.
func NewRemoteWeavelet(ctx context.Context, regs []*codegen.Registration, bootstrap runtime.Bootstrap, opts RemoteWeaveletOptions) (*RemoteWeavelet, error) {
	servers, ctx := errgroup.WithContext(ctx)
	w := &RemoteWeavelet{
		ctx:              ctx,
		servers:          servers,
		opts:             opts,
		logDst:           newRemoteLogger(os.Stderr),
		componentsByName: map[string]*component{},
		componentsByIntf: map[reflect.Type]*component{},
		componentsByImpl: map[reflect.Type]*component{},
		redirects:        map[string]redirect{},
		listeners:        map[string]*listener{},
	}

	// Establish a connection with the envelope.
	toWeavelet, toEnvelope, err := bootstrap.MakePipes()
	if err != nil {
		return nil, err
	}
	// TODO(mwhittaker): Pass handler to Serve, not NewWeaveletConn.
	w.conn, err = conn.NewWeaveletConn(toWeavelet, toEnvelope)
	if err != nil {
		return nil, fmt.Errorf("new weavelet conn: %w", err)
	}
	info := w.conn.EnvelopeInfo()

	// Set up logging.
	w.syslogger = w.logger("weavelet", "serviceweaver/system", "")

	// Set up tracing.
	exporter := traceio.NewWriter(w.conn.SendTraceSpans)
	w.tracer = tracer(exporter, info.App, info.DeploymentId, info.Id)

	// Initialize the component structs.
	for _, reg := range regs {
		c := &component{reg: reg}
		w.componentsByName[reg.Name] = c
		w.componentsByIntf[reg.Iface] = c
		w.componentsByImpl[reg.Impl] = c

		// Initialize the load collector.
		if reg.Routed {
			// TODO(rgrandl): In the future, we may want to collect load for
			// all components.
			c.load = newLoadCollector(reg.Name, w.conn.WeaveletInfo().DialAddr)
		}

		// Initialize the client side of the mTLS protocol.
		if info.Mtls {
			c.clientTLS = &tls.Config{
				GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
					return w.getSelfCertificate()
				},
				InsecureSkipVerify: true, // ok when VerifyPeerCertificate present
				VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
					return w.verifyServerCertificate(rawCerts, reg.Name)
				},
			}
		}

		// Initialize the resolver and balancer.
		c.resolver = newRoutingResolver()
		c.balancer = newRoutingBalancer(c.clientTLS)
	}

	// Process all redirects.
	for _, r := range info.Redirects {
		c, ok := w.componentsByName[r.Component]
		if !ok {
			return nil, fmt.Errorf("redirect names unknown component %q", r.Component)
		}
		w.redirects[r.Component] = redirect{c, r.Target, r.Address}
	}

	// Wire-up log writing.
	logFn, err := w.getLoggerFunction()
	if err != nil {
		return nil, err
	}
	servers.Go(func() error {
		w.logDst.run(ctx, logFn)
		return nil
	})

	// Serve deployer API requests on the weavelet conn.
	servers.Go(func() error {
		if err := w.conn.Serve(ctx, w); err != nil {
			w.syslogger.Error("weavelet conn failed", "err", err)
			return err
		}
		return nil
	})

	// Serve RPC requests from other weavelets.
	servers.Go(func() error {
		server := &server{Listener: w.conn.Listener(), wlet: w}
		opts := call.ServerOptions{
			Logger: w.syslogger,
			Tracer: w.tracer,
		}
		if err := call.Serve(w.ctx, server, opts); err != nil {
			w.syslogger.Error("RPC server failed", "err", err)
			return err
		}
		return nil
	})

	w.syslogger.Debug("🧶 weavelet started", "addr", w.conn.WeaveletInfo().DialAddr)
	return w, nil
}

// Wait waits for the RemoteWeavelet to fully shut down after its context has
// been cancelled.
func (w *RemoteWeavelet) Wait() error {
	return w.servers.Wait()
}

// GetIntf implements the Weavelet interface.
func (w *RemoteWeavelet) GetIntf(t reflect.Type) (any, error) {
	return w.getIntf(t, "root")
}

// getIntf is identical to [GetIntf], but has an additional requester argument
// to track which components request other components.
func (w *RemoteWeavelet) getIntf(t reflect.Type, requester string) (any, error) {
	c, ok := w.componentsByIntf[t]
	if !ok {
		return nil, fmt.Errorf("component of type %v was not registered; maybe you forgot to run weaver generate", t)
	}

	if r, ok := w.redirects[c.reg.Name]; ok {
		return w.redirect(requester, c, r.target, r.address)
	}

	// Activate the component.
	c.activateInit.Do(func() {
		name := logging.ShortenComponent(c.reg.Name)
		w.syslogger.Debug("Activating", "component", name)
		errMsg := fmt.Sprintf("cannot activate component %q", c.reg.Name)
		c.activateErr = w.repeatedly(w.ctx, errMsg, func() error {
			request := &protos.ActivateComponentRequest{
				Component: c.reg.Name,
				Routed:    c.reg.Routed,
			}
			return w.conn.ActivateComponentRPC(request)
		})
		if c.activateErr != nil {
			w.syslogger.Error("Failed to activate", "component", name, "err", c.activateErr)
		} else {
			w.syslogger.Debug("Activated", "component", name)
		}
	})
	if c.activateErr != nil {
		return nil, c.activateErr
	}

	// Return a local stub.
	if c.local.Read() {
		impl, err := w.GetImpl(c.reg.Impl)
		if err != nil {
			return nil, err
		}
		return c.reg.LocalStubFn(impl, requester, w.tracer), nil
	}

	// Return a remote stub.
	stub, err := w.getStub(c)
	if err != nil {
		return nil, err
	}
	return c.reg.ClientStubFn(stub, requester), nil
}

// redirect creates a component interface for c that redirects calls to the
// component named by target at address.
func (w *RemoteWeavelet) redirect(requester string, c *component, target, address string) (any, error) {
	// Assume component is already activated.
	c.activateInit.Do(func() {})

	// Make special stub.
	c.stubInit.Do(func() {
		// Make a constant resolver pointing at address.
		endpoint, err := call.ParseNetEndpoint(address)
		if err != nil {
			c.stubErr = err
			return
		}
		resolver := call.NewConstantResolver(endpoint)
		// TODO(sanjay): Pass retry info from the target component.
		c.stub, c.stubErr = w.makeStub(target, c.reg, resolver, nil)
	})
	if c.stubErr != nil {
		return nil, c.stubErr
	}
	return c.reg.ClientStubFn(c.stub, requester), nil
}

// GetImpl implements the Weavelet interface.
func (w *RemoteWeavelet) GetImpl(t reflect.Type) (any, error) {
	c, ok := w.componentsByImpl[t]
	if !ok {
		return nil, fmt.Errorf("component implementation of type %v was not registered; maybe you forgot to run weaver generate", t)
	}

	c.implInit.Do(func() {
		name := logging.ShortenComponent(c.reg.Name)
		w.syslogger.Debug("Constructing", "component", name)
		c.impl, c.implErr = w.createComponent(w.ctx, c.reg)
		if c.implErr != nil {
			w.syslogger.Error("Failed to construct", "component", name, "err", c.implErr)
			return
		} else {
			w.syslogger.Debug("Constructed", "component", name)
		}

		logger := w.logger(c.reg.Name)
		c.serverStub = c.reg.ServerStubFn(c.impl, func(key uint64, v float64) {
			if c.reg.Routed {
				if err := c.load.add(key, v); err != nil {
					logger.Error("add load", "err", err, "component", c.reg.Name, "key", key)
				}
			}
		})
	})
	return c.impl, c.implErr
}

// createComponent creates a component with the provided registration.
//
// TODO(mwhittaker): Deduplicate with localweavelet.go.
func (w *RemoteWeavelet) createComponent(ctx context.Context, reg *codegen.Registration) (any, error) {
	if obj, ok := w.opts.Fakes[reg.Iface]; ok {
		// We have a fake registered for this component.
		return obj, nil
	}

	// Create the implementation object.
	v := reflect.New(reg.Impl)
	obj := v.Interface()

	// Fill config if necessary.
	if cfg := config.Config(v); cfg != nil {
		if err := runtime.ParseConfigSection(reg.Name, "", w.Info().Sections, cfg); err != nil {
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
		if err != nil {
			return nil, "", err
		}
		return lis.lis, lis.proxyAddr, nil
	}); err != nil {
		return nil, err
	}

	// Call Init if available.
	if i, ok := obj.(interface{ Init(context.Context) error }); ok {
		if err := i.Init(ctx); err != nil {
			return nil, fmt.Errorf("component %q initialization failed: %w", reg.Name, err)
		}
	}
	return obj, nil
}

// getStub returns a component's client stub, initializing it if necessary.
func (w *RemoteWeavelet) getStub(c *component) (*stub, error) {
	c.stubInit.Do(func() {
		c.stub, c.stubErr = w.makeStub(c.reg.Name, c.reg, c.resolver, c.balancer)
	})
	return c.stub, c.stubErr
}

// makeStub makes a new stub with the provided resolver and balancer.
func (w *RemoteWeavelet) makeStub(fullName string, reg *codegen.Registration, resolver call.Resolver, balancer call.Balancer) (*stub, error) {
	// Create the client connection.
	name := logging.ShortenComponent(fullName)
	w.syslogger.Debug("Connecting to remote", "component", name)
	opts := call.ClientOptions{
		Balancer: balancer,
		Logger:   w.syslogger,
	}
	conn, err := call.Connect(w.ctx, resolver, opts)
	if err != nil {
		w.syslogger.Error("Failed to connect to remote", "component", name, "err", err)
		return nil, err
	}
	if err := waitUntilReady(w.ctx, conn); err != nil {
		w.syslogger.Error("Failed to wait for remote", "component", name, "err", err)
		return nil, err
	}
	w.syslogger.Debug("Connected to remote", "component", name)

	return &stub{
		component:     fullName,
		conn:          conn,
		methods:       makeStubMethods(fullName, reg),
		tracer:        w.tracer,
		injectRetries: w.opts.InjectRetries,
	}, nil
}

// GetLoad implements the conn.WeaveletHandler interface.
func (w *RemoteWeavelet) GetLoad(*protos.GetLoadRequest) (*protos.GetLoadReply, error) {
	report := &protos.LoadReport{
		Loads: map[string]*protos.LoadReport_ComponentLoad{},
	}
	for _, c := range w.componentsByName {
		if c.load == nil {
			continue
		}
		if x := c.load.report(); x != nil {
			report.Loads[c.reg.Name] = x
		}
		c.load.reset()
	}
	return &protos.GetLoadReply{Load: report}, nil
}

// UpdateComponents implements the conn.WeaverHandler interface.
func (w *RemoteWeavelet) UpdateComponents(req *protos.UpdateComponentsRequest) (*protos.UpdateComponentsReply, error) {
	var errs []error
	var components []*component
	var shortened []string
	for _, component := range req.Components {
		short := logging.ShortenComponent(component)
		shortened = append(shortened, short)
		c, err := w.getComponent(component)
		if err != nil {
			w.syslogger.Error("Failed to update", "component", short, "err", err)
			errs = append(errs, err)
			continue
		}
		components = append(components, c)
	}

	// Create components in a separate goroutine. A component's Init function
	// may be slow or block. It may also trigger pipe communication. We want to
	// avoid blocking and pipe communication in this handler as it could cause
	// deadlocks in a deployer.
	//
	// TODO(mwhittaker): Document that handlers shouldn't retain access to the
	// arguments passed to them.
	for i, c := range components {
		i := i
		c := c
		go func() {
			w.syslogger.Debug("Updating", "components", shortened[i])
			if _, err := w.GetImpl(c.reg.Impl); err != nil {
				// TODO(mwhittaker): Propagate errors.
				w.syslogger.Error("Failed to update", "component", shortened[i], "err", err)
				return
			}
			w.syslogger.Debug("Updated", "component", shortened[i])
		}()
	}

	return &protos.UpdateComponentsReply{}, errors.Join(errs...)
}

// UpdateRoutingInfo implements the conn.WeaverHandler interface.
func (w *RemoteWeavelet) UpdateRoutingInfo(req *protos.UpdateRoutingInfoRequest) (reply *protos.UpdateRoutingInfoReply, err error) {
	if req.RoutingInfo == nil {
		w.syslogger.Error("Failed to update nil routing info")
		return nil, fmt.Errorf("nil RoutingInfo")
	}
	info := req.RoutingInfo

	defer func() {
		name := logging.ShortenComponent(info.Component)
		routing := fmt.Sprint(info.Replicas)
		if info.Local {
			routing = "local"
		}
		if err != nil {
			w.syslogger.Error("Failed to update routing info", "component", name, "addr", routing, "err", err)
		} else {
			w.syslogger.Debug("Updated routing info", "component", name, "addr", routing)
		}
	}()

	c, err := w.getComponent(info.Component)
	if err != nil {
		return nil, err
	}

	// Record whether the component is local or remote. Currently, a component
	// must always be local or always be remote. It cannot change.
	c.local.TryWrite(info.Local)
	if got, want := c.local.Read(), info.Local; got != want {
		return nil, fmt.Errorf("RoutingInfo.Local for %q: got %t, want %t", info.Component, got, want)
	}

	// If the component is local, we don't have to update anything. The routing
	// info shouldn't contain any replicas or assignment.
	if info.Local {
		if len(info.Replicas) > 0 {
			w.syslogger.Error("Local routing info has replicas", "component", info.Component, "replicas", info.Replicas)
		}
		if info.Assignment != nil {
			w.syslogger.Error("Local routing info has assignment", "component", info.Component, "assignment", info.Assignment)
		}
		return
	}

	// Update resolver.
	endpoints, err := parseEndpoints(info.Replicas, c.clientTLS)
	if err != nil {
		return nil, err
	}
	c.resolver.update(endpoints)

	// Update balancer.
	if info.Assignment != nil {
		c.balancer.update(info.Assignment)
	}

	// Update load collector.
	if c.load != nil && info.Assignment != nil {
		c.load.updateAssignment(info.Assignment)
	}

	return &protos.UpdateRoutingInfoReply{}, nil
}

// Info returns the EnvelopeInfo received from the envelope.
func (w *RemoteWeavelet) Info() *protos.EnvelopeInfo {
	return w.conn.EnvelopeInfo()
}

// getComponent returns the component with the given name.
func (w *RemoteWeavelet) getComponent(name string) (*component, error) {
	// Note that we don't need to lock w.components because, while the
	// components within w.components are modified, w.components itself is
	// read-only.
	c, ok := w.componentsByName[name]
	if !ok {
		return nil, fmt.Errorf("component %q was not registered; maybe you forgot to run weaver generate", name)
	}
	return c, nil
}

// addHandlers registers a component's methods as handlers in the given map.
// Specifically, for every method m in the component, we register a function f
// that (1) creates the local component if it hasn't been created yet and (2)
// calls m.
func (w *RemoteWeavelet) addHandlers(handlers *call.HandlerMap, c *component) {
	for i, n := 0, c.reg.Iface.NumMethod(); i < n; i++ {
		mname := c.reg.Iface.Method(i).Name
		handler := func(ctx context.Context, args []byte) (res []byte, err error) {
			// This handler is supposed to invoke the method named mname on the
			// local component. However, it is possible that the component has
			// not yet been started. w.GetImpl will start the component if it
			// hasn't already been started, or it will be a noop if the
			// component has already been started.
			if _, err := w.GetImpl(c.reg.Impl); err != nil {
				return nil, err
			}
			fn := c.serverStub.GetStubFn(mname)
			return fn(ctx, args)
		}
		handlers.Set(c.reg.Name, mname, handler)
	}
}

// repeatedly repeatedly executes f until it succeeds or until ctx is cancelled.
func (w *RemoteWeavelet) repeatedly(ctx context.Context, errMsg string, f func() error) error {
	for r := retry.Begin(); r.Continue(ctx); {
		if err := f(); err != nil {
			w.syslogger.Error(errMsg+"; will retry", "err", err)
			continue
		}
		return nil
	}
	return fmt.Errorf("%s: %w", errMsg, ctx.Err())
}

func (w *RemoteWeavelet) getLoggerFunction() (func(context.Context, *protos.LogEntryBatch) error, error) {
	// If an override is found for the logger component, use it.
	const loggerPath = "github.com/ServiceWeaver/weaver/Logger"
	r, ok := w.redirects[loggerPath]
	if !ok {
		// For now, fall back to sending over the pipe to the weavelet.
		// TODO(sanjay): Make the default write to os.Stderr once all deployers
		// provide a logging component.
		return func(ctx context.Context, batch *protos.LogEntryBatch) error {
			for _, e := range batch.Entries {
				if err := w.conn.SendLogEntry(e); err != nil {
					return err
				}
			}
			return nil
		}, nil
	}

	comp, err := w.getIntf(r.component.reg.Iface, r.target)
	if err != nil {
		return nil, err
	}
	loggerComponent, ok := comp.(interface {
		LogBatch(context.Context, *protos.LogEntryBatch) error
	})
	if !ok {
		return nil, fmt.Errorf("redirected component of type %T is not a weaver.Logger", comp)
	}
	return loggerComponent.LogBatch, nil
}

// logger returns a logger for the component with the provided name. The
// returned logger includes the provided attributes.
func (w *RemoteWeavelet) logger(name string, attrs ...string) *slog.Logger {
	return slog.New(&logging.LogHandler{
		Opts: logging.Options{
			App:        w.Info().App,
			Deployment: w.Info().DeploymentId,
			Component:  name,
			Weavelet:   w.Info().Id,
			Attrs:      attrs,
		},
		Write: w.logDst.log,
	})
}

// listener returns the listener with the provided name.
func (w *RemoteWeavelet) listener(name string) (*listener, error) {
	w.lismu.Lock()
	defer w.lismu.Unlock()
	if lis, ok := w.listeners[name]; ok {
		// The listener already exists.
		return lis, nil
	}

	if name == "" {
		return nil, fmt.Errorf("listener(%q): empty listener name", name)
	}

	// Get the address to listen on.
	addr, err := w.getListenerAddress(name)
	if err != nil {
		return nil, fmt.Errorf("listener(%q): %w", name, err)
	}

	// Listen on the address.
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listener(%q): %w", name, err)
	}

	// Export the listener.
	errMsg := fmt.Sprintf("listener(%q): error exporting listener %v", name, lis.Addr())
	var reply *protos.ExportListenerReply
	if err := w.repeatedly(w.ctx, errMsg, func() error {
		var err error
		request := &protos.ExportListenerRequest{
			Listener: name,
			Address:  lis.Addr().String(),
		}
		reply, err = w.conn.ExportListenerRPC(request)
		return err
	}); err != nil {
		return nil, err
	}
	if reply.Error != "" {
		return nil, fmt.Errorf("listener(%q): %s", name, reply.Error)
	}

	// Store the listener.
	l := &listener{lis, reply.ProxyAddress}
	w.listeners[name] = l
	return l, nil
}

func (w *RemoteWeavelet) getListenerAddress(name string) (string, error) {
	request := &protos.GetListenerAddressRequest{Name: name}
	reply, err := w.conn.GetListenerAddressRPC(request)
	if err != nil {
		return "", err
	}
	return reply.Address, nil
}

func (w *RemoteWeavelet) getSelfCertificate() (*tls.Certificate, error) {
	request := &protos.GetSelfCertificateRequest{}
	reply, err := w.conn.GetSelfCertificateRPC(request)
	if err != nil {
		return nil, err
	}
	tlsCert, err := tls.X509KeyPair(reply.Cert, reply.Key)
	if err != nil {
		return nil, err
	}
	return &tlsCert, nil
}

func (w *RemoteWeavelet) verifyClientCertificate(certChain [][]byte) ([]string, error) {
	request := &protos.VerifyClientCertificateRequest{CertChain: certChain}
	reply, err := w.conn.VerifyClientCertificateRPC(request)
	if err != nil {
		return nil, err
	}
	return reply.Components, nil
}

func (w *RemoteWeavelet) verifyServerCertificate(certChain [][]byte, targetComponent string) error {
	request := &protos.VerifyServerCertificateRequest{
		CertChain:       certChain,
		TargetComponent: targetComponent,
	}
	return w.conn.VerifyServerCertificateRPC(request)
}

// server serves RPC traffic from other RemoteWeavelets.
type server struct {
	net.Listener
	wlet *RemoteWeavelet
}

var _ call.Listener = &server{}

// Accept implements the call.Listener interface.
func (s *server) Accept() (net.Conn, *call.HandlerMap, error) {
	conn, err := s.Listener.Accept()
	if err != nil {
		return nil, nil, err
	}

	if !s.wlet.Info().Mtls {
		// No security: all components are accessible.
		hm, err := s.handlers(maps.Keys(s.wlet.componentsByName))
		return conn, hm, err
	}

	// Establish a TLS connection with the client and get the list of
	// components it can access.
	var accessibleComponents []string
	tlsConfig := &tls.Config{
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return s.wlet.getSelfCertificate()
		},
		ClientAuth: tls.RequireAnyClientCert,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			var err error
			accessibleComponents, err = s.wlet.verifyClientCertificate(rawCerts)
			return err
		},
	}
	tlsConn := tls.Server(conn, tlsConfig)
	if err := tlsConn.HandshakeContext(s.wlet.ctx); err != nil {
		return nil, nil, fmt.Errorf("TLS handshake error: %w", err)
	}

	// NOTE: VerifyPeerCertificate above has been called at this point.
	hm, err := s.handlers(accessibleComponents)
	return tlsConn, hm, err
}

// handlers returns method handlers for the given components.
func (s *server) handlers(components []string) (*call.HandlerMap, error) {
	// Note that the components themselves may not be started, but we still
	// register their handlers to avoid concurrency issues with on-demand
	// handler additions.
	hm := call.NewHandlerMap()
	for _, component := range components {
		c, err := s.wlet.getComponent(component)
		if err != nil {
			return nil, err
		}
		s.wlet.addHandlers(hm, c)
	}
	return hm, nil
}

// waitUntilReady blocks until a successful call to the "ready" method is made
// on the provided client.
func waitUntilReady(ctx context.Context, client call.Connection) error {
	for r := retry.Begin(); r.Continue(ctx); {
		_, err := client.Call(ctx, readyMethodKey, nil, call.CallOptions{})
		if err == nil || !errors.Is(err, call.Unreachable) {
			return err
		}
	}
	return ctx.Err()
}

// parseEndpoints parses a list of endpoint addresses into a list of
// call.Endpoints.
func parseEndpoints(addrs []string, config *tls.Config) ([]call.Endpoint, error) {
	var endpoints []call.Endpoint
	var err error
	var ep call.Endpoint
	for _, addr := range addrs {
		const mtlsPrefix = "mtls://"
		if ep, err = call.ParseNetEndpoint(strings.TrimPrefix(addr, mtlsPrefix)); err != nil {
			return nil, err
		}
		if strings.HasPrefix(addr, mtlsPrefix) {
			if config == nil {
				return nil, fmt.Errorf("mtls protocol requires a non-nil TLS config")
			}
			ep = call.MTLS(config, ep)
		}
		endpoints = append(endpoints, ep)
	}
	return endpoints, nil
}
