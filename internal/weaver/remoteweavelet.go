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
	"net"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

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
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
)

// readyMethodKey holds the key for a method used to check if a backend is ready.
var readyMethodKey = call.MakeMethodKey("", "ready")

// RemoteWeaveletOptions configure a RemoteWeavelet.
type RemoteWeaveletOptions struct {
	Fakes map[reflect.Type]any // component fakes, by component interface type
}

// RemoteWeavelet is a weavelet that runs some components locally, but
// coordinates with a deployer over a set of Unix pipes to start other
// components remotely. It is the weavelet used by all deployers, except for
// the single process deployer.
type RemoteWeavelet struct {
	ctx       context.Context       // shuts down the weavelet when canceled
	opts      RemoteWeaveletOptions // options
	conn      *conn.WeaveletConn    // connection to envelope
	syslogger *slog.Logger          // system logger
	tracer    trace.Tracer          // tracer used by all components

	componentsByName map[string]*component       // component name -> component
	componentsByIntf map[reflect.Type]*component // component interface type -> component
	componentsByImpl map[reflect.Type]*component // component impl type -> component

	lismu     sync.Mutex           // guards listeners
	listeners map[string]*listener // listeners, by name
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
	w := &RemoteWeavelet{
		ctx:              ctx,
		opts:             opts,
		componentsByName: map[string]*component{},
		componentsByIntf: map[reflect.Type]*component{},
		componentsByImpl: map[reflect.Type]*component{},
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
	w.syslogger = w.logger(fmt.Sprintf("weavelet-%s", logging.Shorten(info.Id)), "serviceweaver/system", "")

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

	// Serve deployer API requests on the weavelet conn.
	runAndDie(ctx, "serve weavelet conn", func() error {
		return w.conn.Serve(w)
	})

	// Serve RPC requests from other weavelets.
	server := &server{Listener: w.conn.Listener(), wlet: w}
	runAndDie(w.ctx, "handle calls", func() error {
		opts := call.ServerOptions{
			Logger:                w.syslogger,
			Tracer:                w.tracer,
			InlineHandlerDuration: 20 * time.Microsecond,
			WriteFlattenLimit:     4 << 10,
		}
		return call.Serve(w.ctx, server, opts)
	})

	w.logRolodexCard()
	return w, nil
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

	// Activate the component.
	c.activateInit.Do(func() {
		w.syslogger.Debug("Activating component...", "component", c.reg.Name)
		errMsg := fmt.Sprintf("cannot activate component %q", c.reg.Name)
		c.activateErr = w.repeatedly(w.ctx, errMsg, func() error {
			request := &protos.ActivateComponentRequest{
				Component: c.reg.Name,
				Routed:    c.reg.Routed,
			}
			return w.conn.ActivateComponentRPC(request)
		})
		if c.activateErr != nil {
			w.syslogger.Error("Activating component failed", "err", c.activateErr, "component", c.reg.Name)
		} else {
			w.syslogger.Debug("Activating component succeeded", "component", c.reg.Name)
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

// GetImpl implements the Weavelet interface.
func (w *RemoteWeavelet) GetImpl(t reflect.Type) (any, error) {
	c, ok := w.componentsByImpl[t]
	if !ok {
		return nil, fmt.Errorf("component implementation of type %v was not registered; maybe you forgot to run weaver generate", t)
	}

	c.implInit.Do(func() {
		w.syslogger.Debug("Constructing component", "component", c.reg.Name)
		c.impl, c.implErr = w.createComponent(w.ctx, c.reg)
		if c.implErr != nil {
			w.syslogger.Error("Constructing component failed", "err", c.implErr, "component", c.reg.Name)
			return
		} else {
			w.syslogger.Debug("Constructing component succeeded", "component", c.reg.Name)
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
		c.stub, c.stubErr = w.makeStub(c.reg, c.resolver, c.balancer)
	})
	return c.stub, c.stubErr
}

// makeStub makes a new stub with the provided resolver and balancer.
func (w *RemoteWeavelet) makeStub(reg *codegen.Registration, resolver *routingResolver, balancer *routingBalancer) (*stub, error) {
	// Create the client connection.
	w.syslogger.Debug("Creating a connection to a remote component...", "component", reg.Name)
	opts := call.ClientOptions{
		Logger:            w.syslogger,
		WriteFlattenLimit: 4 << 10,
	}
	conn, err := call.Connect(w.ctx, resolver, opts)
	if err != nil {
		w.syslogger.Error("Creating a connection to remote component failed", "err", err, "component", reg.Name)
		return nil, err
	}
	if err := waitUntilReady(w.ctx, conn); err != nil {
		w.syslogger.Error("Waiting for remote component failed", "err", err, "component", reg.Name)
		return nil, err
	}
	w.syslogger.Debug("Creating connection to remote component succeeded", "component", reg.Name)

	// Construct the keys for the methods.
	n := reg.Iface.NumMethod()
	methods := make([]call.MethodKey, n)
	for i := 0; i < n; i++ {
		mname := reg.Iface.Method(i).Name
		methods[i] = call.MakeMethodKey(reg.Name, mname)
	}

	return &stub{
		component: reg.Name,
		conn:      conn,
		methods:   methods,
		balancer:  balancer,
		tracer:    w.tracer,
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
	// Create components in a separate goroutine. A component's Init function
	// may be slow or block. It may also trigger pipe communication. We want to
	// avoid blocking and pipe communication in this handler as it could cause
	// deadlocks in a deployer.
	//
	// TODO(mwhittaker): Start every component in its own goroutine? This way,
	// constructors that block don't prevent other components from starting.
	//
	// TODO(mwhittaker): Document that handlers shouldn't retain access to the
	// arguments passed to them.
	components := slices.Clone(req.Components)
	w.syslogger.Debug("UpdateComponents", "components", components)
	go func() {
		for _, component := range components {
			c, err := w.getComponent(component)
			if err != nil {
				// TODO(mwhittaker): Propagate errors.
				w.syslogger.Error("getComponent", "err", err, "component", component)
				return
			}
			if _, err = w.GetImpl(c.reg.Impl); err != nil {
				// TODO(mwhittaker): Propagate errors.
				w.syslogger.Error("GetImpl", "err", err, "component", component)
				return
			}
		}
	}()
	return &protos.UpdateComponentsReply{}, nil
}

// UpdateRoutingInfo implements the conn.WeaverHandler interface.
func (w *RemoteWeavelet) UpdateRoutingInfo(req *protos.UpdateRoutingInfoRequest) (reply *protos.UpdateRoutingInfoReply, err error) {
	logger := w.syslogger.With(
		"component", req.RoutingInfo.Component,
		"local", req.RoutingInfo.Local,
		"replicas", req.RoutingInfo.Replicas,
	)

	logger.Debug("Updating routing info...")
	defer func() {
		if err != nil {
			logger.Error("Updating routing info failed", "err", err)
		} else {
			logger.Debug("Updating routing info succeeded")
		}
	}()

	// Update load collector.
	for _, c := range w.componentsByName {
		// TODO(mwhittaker): Double check this.
		if c.load != nil && req.RoutingInfo.Assignment != nil {
			c.load.updateAssignment(req.RoutingInfo.Assignment)
		}
	}

	c, err := w.getComponent(req.RoutingInfo.Component)
	if err != nil {
		return nil, err
	}

	// Update resolver and balancer.
	endpoints, err := parseEndpoints(req.RoutingInfo.Replicas, c.clientTLS)
	if err != nil {
		return nil, err
	}
	c.resolver.update(endpoints)
	c.balancer.update(req.RoutingInfo.Assignment)

	// Update local.
	c.local.TryWrite(req.RoutingInfo.Local)
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
		Write: func(entry *protos.LogEntry) {
			// TODO(mwhittaker): Propagate error.
			w.conn.SendLogEntry(entry) //nolint:errcheck
		},
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

// logRolodexCard pretty prints a card that includes basic information about
// the weavelet. It looks something like this:
//
//	┌ weavelet 5b2d9d03-d21e-4ae9-a875-eab80af85350 started ┐
//	│   hostname   : alan.turing.com                        │
//	│   deployment : f20bbe05-85a5-4596-bab6-60e75b366306   │
//	│   address:   : tcp://127.0.0.1:43937                  │
//	│   pid        : 836347                                 │
//	└───────────────────────────────────────────────────────┘
func (w *RemoteWeavelet) logRolodexCard() {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "UNKNOWN"
	}

	header := fmt.Sprintf(" weavelet %s started ", w.Info().Id)
	lines := []string{
		fmt.Sprintf("   hostname   : %s ", hostname),
		fmt.Sprintf("   deployment : %s ", w.Info().DeploymentId),
		fmt.Sprintf("   address    : %s", w.conn.WeaveletInfo().DialAddr),
		fmt.Sprintf("   pid        : %v ", os.Getpid()),
	}

	width := len(header)
	for _, line := range lines {
		if len(line) > width {
			width = len(line)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n┌%s%s┐\n", header, strings.Repeat("─", width-len(header)))
	for _, line := range lines {
		fmt.Fprintf(&b, "│%*s│\n", -width, line)
	}
	fmt.Fprintf(&b, "└%s┘", strings.Repeat("─", width))
	w.syslogger.Debug(b.String())
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
	hm := &call.HandlerMap{}
	for _, component := range components {
		c, err := s.wlet.getComponent(component)
		if err != nil {
			return nil, err
		}
		s.wlet.addHandlers(hm, c)
	}

	// Add a dummy "ready" handler. Clients will repeatedly call this
	// RPC until it responds successfully, ensuring the server is ready.
	hm.Set("", "ready", func(context.Context, []byte) ([]byte, error) {
		return nil, nil
	})
	return hm, nil
}

// runAndDie runs fn in the background. Errors are fatal unless ctx has been
// canceled.
func runAndDie(ctx context.Context, msg string, fn func() error) {
	go func() {
		if err := fn(); err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err)
			os.Exit(1)
		}
	}()
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
