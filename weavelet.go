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
	"github.com/ServiceWeaver/weaver/internal/private"
	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
)

// readyMethodKey holds the key for a method used to check if a backend is ready.
var readyMethodKey = call.MakeMethodKey("", "ready")

// A weavelet runs and manages components. As the name suggests, a weavelet is
// analogous to a kubelet.
type weavelet struct {
	ctx       context.Context
	env       env                  // Manages interactions with execution environment
	info      *protos.EnvelopeInfo // Setup info sent by the deployer.
	transport *transport           // Transport for cross-weavelet communication
	dialAddr  string               // Address this weavelet is reachable at
	tracer    trace.Tracer         // Tracer for this weavelet
	overrides map[reflect.Type]any // Component implementation overrides

	componentsByName map[string]*component       // component name -> component
	componentsByType map[reflect.Type]*component // component type -> component

	listenersMu sync.Mutex
	listeners   map[string]*listenerState
}

type listenerState struct {
	addr        string
	initialized chan struct{} // Closed when addr has been filled
}

type transport struct {
	clientOpts call.ClientOptions
	serverOpts call.ServerOptions
}

type client struct {
	resolver *routingResolver
	balancer *routingBalancer
}

type server struct {
	net.Listener
	wlet *weavelet
}

// Ensure that WeaveletHandler remains in-sync with conn.WeaveletHandler.
var _ conn.WeaveletHandler = &weavelet{}

// weavelet should also implement the private.App API used by weavertest.
var _ private.App = &weavelet{}

// newWeavelet returns a new weavelet.
func newWeavelet(ctx context.Context, options private.AppOptions, componentInfos []*codegen.Registration) (*weavelet, error) {
	w := &weavelet{
		ctx:              ctx,
		overrides:        options.Fakes,
		componentsByName: make(map[string]*component, len(componentInfos)),
		componentsByType: make(map[reflect.Type]*component, len(componentInfos)),
	}

	// TODO(mwhittaker): getEnv starts the WeaveletConn handler which calls
	// methods of w, but w hasn't yet been fully constructed. This is a race.
	env, err := getEnv(ctx, w)
	if err != nil {
		return nil, err
	}
	w.env = env

	info := env.EnvelopeInfo()
	if info == nil {
		return nil, fmt.Errorf("unable to get weavelet information")
	}
	w.info = info

	for _, info := range componentInfos {
		c := &component{
			wlet: w,
			info: info,
			// May be remote, so start with no-op logger. May set real logger later.
			// Discard all log entries.
			logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError + 1})),
		}
		w.componentsByName[info.Name] = c
		w.componentsByType[info.Iface] = c
	}

	if info.Mtls {
		// Initialize client side of the mTLS protocol.
		for cname, c := range w.componentsByName {
			cname := cname
			c.clientTLS = &tls.Config{
				GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
					return w.getSelfCertificate()
				},
				InsecureSkipVerify: true, // ok when VerifyPeerCertificate present
				VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
					return w.env.VerifyServerCertificate(w.ctx, rawCerts, cname)
				},
			}
		}
	}

	const instrumentationLibrary = "github.com/ServiceWeaver/weaver/serviceweaver"
	const instrumentationVersion = "0.0.1"
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(env.CreateTraceExporter()),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(fmt.Sprintf("serviceweaver/%s", info.Id)),
			semconv.ProcessPIDKey.Int(os.Getpid()),
			traceio.AppTraceKey.String(info.App),
			traceio.DeploymentIdTraceKey.String(info.DeploymentId),
			traceio.WeaveletIdTraceKey.String(info.Id),
		)),
		// TODO(spetrovic): Allow the user to create new TracerProviders where
		// they can control trace sampling and other options.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())))
	tracer := tracerProvider.Tracer(instrumentationLibrary, trace.WithInstrumentationVersion(instrumentationVersion))

	// Set global tracing defaults.
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	w.transport = &transport{
		clientOpts: call.ClientOptions{
			Logger:            env.SystemLogger(),
			WriteFlattenLimit: 4 << 10,
		},
		serverOpts: call.ServerOptions{
			Logger:                env.SystemLogger(),
			Tracer:                tracer,
			InlineHandlerDuration: 20 * time.Microsecond,
			WriteFlattenLimit:     4 << 10,
		},
	}
	w.tracer = tracer
	return w, nil
}

// getSelfCertificate returns the certificate the weavelet should use for
// establishing a network connection. Only called if w.info.Mtls.
func (w *weavelet) getSelfCertificate() (*tls.Certificate, error) {
	cert, key, err := w.env.GetSelfCertificate(w.ctx)
	if err != nil {
		return nil, err
	}
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	return &tlsCert, nil
}

// start starts a weavelet, executing the logic to start and manage components.
func (w *weavelet) start() error {
	// Launch status server for single process deployments.
	if single, ok := w.env.(*singleprocessEnv); ok {
		go func() {
			if err := single.serveStatus(w.ctx); err != nil {
				single.SystemLogger().Error("status server", "err", err)
			}
		}()
	}

	if w.info.SingleProcess {
		for _, c := range w.componentsByName {
			// Mark all components as local.
			c.local.TryWrite(true)
		}
	}

	// For a singleprocess deployment, no server is launched because all
	// method invocations are process-local and executed as regular go function
	// calls.
	if remote, ok := w.env.(*remoteEnv); ok {
		startWork(w.ctx, "serve weavelet conn", remote.conn.Serve)

		lis := remote.conn.Listener()
		w.dialAddr = remote.conn.WeaveletInfo().DialAddr
		for _, c := range w.componentsByName {
			if c.info.Routed {
				// TODO(rgrandl): In the future, we may want to collect load for all components.
				c.load = newLoadCollector(c.info.Name, w.dialAddr)
			}
		}

		server := &server{Listener: lis, wlet: w}
		startWork(w.ctx, "handle calls", func() error {
			return call.Serve(w.ctx, server, w.transport.serverOpts)
		})
	}

	w.logRolodexCard()

	// Make sure Main is initialized if local.
	if _, err := w.getMainIfLocal(); err != nil {
		return err
	}
	return nil
}

// getMainIfLocal returns the weaver.Main implementation if hosted in
// this weavelet, or nil if weaver.Main is remote.
func (w *weavelet) getMainIfLocal() (*componentImpl, error) {
	// Note that a weavertest may have RunMain set to true, but no main
	// component registered.
	if m, ok := w.componentsByType[reflection.Type[Main]()]; ok && w.info.RunMain {
		return w.getImpl(w.ctx, m)
	}
	return nil, nil
}

func (w *weavelet) Wait(ctx context.Context) error {
	// Call weaver.Main.Main if weaver.Main is hosted locally.
	impl, err := w.getMainIfLocal()
	if err != nil {
		return nil
	}
	if impl != nil {
		return impl.impl.(Main).Main(ctx)
	}

	// Run until cancelled.
	<-ctx.Done()
	return ctx.Err()
}

func (w *weavelet) Get(requester string, compType reflect.Type) (any, error) {
	component, err := w.getComponentByType(compType)
	if err != nil {
		return nil, err
	}
	result, _, err := w.getInstance(w.ctx, component, requester)
	if err != nil {
		return nil, err
	}
	return result, nil
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
func (w *weavelet) logRolodexCard() {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "UNKNOWN"
	}

	header := fmt.Sprintf(" weavelet %s started ", w.info.Id)
	lines := []string{
		fmt.Sprintf("   hostname   : %s ", hostname),
		fmt.Sprintf("   deployment : %s ", w.info.DeploymentId),
		fmt.Sprintf("   address    : %s", w.dialAddr),
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
	w.env.SystemLogger().Debug(b.String())
}

// getInstance returns an instance of the provided component. If the component
// is local, the results are a local stub used to invoke methods on the component
// as well as the actual local object. Otherwise, the results are a network client
// and nil.
func (w *weavelet) getInstance(ctx context.Context, c *component, requester string) (any, any, error) {
	// Register the component.
	c.registerInit.Do(func() {
		w.env.SystemLogger().Debug("Activating component...", "component", c.info.Name)
		errMsg := fmt.Sprintf("cannot activate component %q", c.info.Name)
		c.registerErr = w.repeatedly(errMsg, func() error {
			return w.env.ActivateComponent(ctx, c.info.Name, c.info.Routed)
		})
		if c.registerErr != nil {
			w.env.SystemLogger().Error("Activating component failed", "err", c.registerErr, "component", c.info.Name)
		} else {
			w.env.SystemLogger().Debug("Activating component succeeded", "component", c.info.Name)
		}
	})
	if c.registerErr != nil {
		return nil, nil, c.registerErr
	}

	if c.local.Read() {
		impl, err := w.getImpl(ctx, c)
		if err != nil {
			return nil, nil, err
		}
		return c.info.LocalStubFn(impl.impl, impl.component.tracer), impl.impl, nil
	}

	stub, err := w.getStub(c)
	if err != nil {
		return nil, nil, err
	}
	return c.info.ClientStubFn(stub, requester), nil, nil
}

// getListener returns a network listener with the given name, along with its
// proxy address.
func (w *weavelet) getListener(name string) (net.Listener, string, error) {
	if name == "" {
		return nil, "", fmt.Errorf("getListener(%q): empty listener name", name)
	}

	// Get the address to listen on.
	addr, err := w.env.GetListenerAddress(w.ctx, name)
	if err != nil {
		return nil, "", fmt.Errorf("getListener(%q): %w", name, err)
	}

	// Listen on the address.
	l, err := net.Listen("tcp", addr.Address)
	if err != nil {
		return nil, "", fmt.Errorf("getListener(%q): %w", name, err)
	}

	// Export the listener.
	errMsg := fmt.Sprintf("getListener(%q): error exporting listener %v", name, l.Addr())
	var reply *protos.ExportListenerReply
	if err := w.repeatedly(errMsg, func() error {
		var err error
		reply, err = w.env.ExportListener(w.ctx, name, l.Addr().String())
		return err
	}); err != nil {
		return nil, "", err
	}
	if reply.Error != "" {
		return nil, "", fmt.Errorf("getListener(%q): %s", name, reply.Error)
	}

	w.listenersMu.Lock()
	defer w.listenersMu.Unlock()
	ls := w.getListenerState(name)
	ls.addr = l.Addr().String()
	close(ls.initialized) // Mark as initialized

	return l, reply.ProxyAddress, nil
}

// addHandlers registers a component's methods as handlers in the given map.
// Specifically, for every method m in the component, we register a function f
// that (1) creates the local component if it hasn't been created yet and (2)
// calls m.
func (w *weavelet) addHandlers(handlers *call.HandlerMap, c *component) {
	for i, n := 0, c.info.Iface.NumMethod(); i < n; i++ {
		mname := c.info.Iface.Method(i).Name
		handler := func(ctx context.Context, args []byte) (res []byte, err error) {
			// This handler is supposed to invoke the method named mname on the
			// local component. However, it is possible that the component has not
			// yet been started (e.g., the start command was issued but hasn't
			// yet taken effect). d.getImpl(c) will start the component if it
			// hasn't already been started, or it will be a noop if the component
			// has already been started.
			impl, err := w.getImpl(w.ctx, c)
			if err != nil {
				return nil, err
			}
			fn := impl.serverStub.GetStubFn(mname)
			return fn(ctx, args)
		}
		handlers.Set(c.info.Name, mname, handler)
	}
}

func (w *weavelet) ListenerAddress(name string) (string, error) {
	w.listenersMu.Lock()
	ls := w.getListenerState(name)
	w.listenersMu.Unlock()

	<-ls.initialized // Wait until initialized
	return ls.addr, nil
}

// REQUIRES: w.listenersMu is held
func (w *weavelet) getListenerState(name string) *listenerState {
	l := w.listeners[name]
	if l != nil {
		return l
	}
	l = &listenerState{initialized: make(chan struct{})}
	if w.listeners == nil {
		w.listeners = map[string]*listenerState{}
	}
	w.listeners[name] = l
	return l
}

// GetLoad implements the WeaveletHandler interface.
func (w *weavelet) GetLoad(*protos.GetLoadRequest) (*protos.GetLoadReply, error) {
	report := &protos.LoadReport{
		Loads: map[string]*protos.LoadReport_ComponentLoad{},
	}

	for _, c := range w.componentsByName {
		if c.load == nil {
			continue
		}
		if x := c.load.report(); x != nil {
			report.Loads[c.info.Name] = x
		}
		// TODO(mwhittaker): If ReportLoad down below fails, we
		// likely don't want to reset our load.
		c.load.reset()
	}
	return &protos.GetLoadReply{Load: report}, nil
}

// UpdateComponents implements the conn.WeaverHandler interface.
func (w *weavelet) UpdateComponents(req *protos.UpdateComponentsRequest) (*protos.UpdateComponentsReply, error) {
	// Create components in a separate goroutine. A component's Init function
	// may be slow or block. It may also call weaver.Get which will trigger
	// pipe communication. We want to avoid blocking and pipe communication in
	// this handler as it could cause deadlocks in a deployer.
	//
	// TODO(mwhittaker): Start every component in its own goroutine? This way,
	// constructors that block don't prevent other components from starting.
	//
	// TODO(mwhittaker): Document that handlers shouldn't retain access to the
	// arguments passed to them.
	components := slices.Clone(req.Components)
	w.env.SystemLogger().Debug("UpdateComponents", "components", components)
	go func() {
		for _, component := range components {
			c, err := w.getComponent(component)
			if err != nil {
				// TODO(mwhittaker): Propagate errors.
				w.env.SystemLogger().Error("getComponent", "err", err, "component", component)
				return
			}
			if _, err = w.getImpl(w.ctx, c); err != nil {
				// TODO(mwhittaker): Propagate errors.
				w.env.SystemLogger().Error("getImpl", "err", err, "component", component)
				return
			}
		}
	}()
	return &protos.UpdateComponentsReply{}, nil
}

// UpdateRoutingInfo implements the conn.WeaverHandler interface.
func (w *weavelet) UpdateRoutingInfo(req *protos.UpdateRoutingInfoRequest) (reply *protos.UpdateRoutingInfoReply, err error) {
	logger := w.env.SystemLogger().With(
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
		if c.load != nil && req.RoutingInfo.Assignment != nil {
			c.load.updateAssignment(req.RoutingInfo.Assignment)
		}
	}

	c, err := w.getComponent(req.RoutingInfo.Component)
	if err != nil {
		return nil, err
	}

	// Update resolver and balancer.
	client := w.getClient(c)
	endpoints, err := parseEndpoints(req.RoutingInfo.Replicas, c.clientTLS)
	if err != nil {
		return nil, err
	}
	client.resolver.update(endpoints)
	client.balancer.update(req.RoutingInfo.Assignment)

	// Update local.
	c.local.TryWrite(req.RoutingInfo.Local)
	return &protos.UpdateRoutingInfoReply{}, nil
}

// getComponent returns the component with the given name.
func (w *weavelet) getComponent(name string) (*component, error) {
	// Note that we don't need to lock d.components because, while the components
	// within d.components are modified, d.components itself is read-only.
	c, ok := w.componentsByName[name]
	if !ok {
		return nil, fmt.Errorf("component %q was not registered; maybe you forgot to run weaver generate", name)
	}
	return c, nil
}

// getComponentByType returns the component with the given type.
func (w *weavelet) getComponentByType(t reflect.Type) (*component, error) {
	// Note that we don't need to lock d.byType because, while the components
	// referenced by d.byType are modified, d.byType itself is read-only.
	c, ok := w.componentsByType[t]
	if !ok {
		return nil, fmt.Errorf("component of type %v was not registered; maybe you forgot to run weaver generate", t)
	}
	return c, nil
}

// getImpl returns a component's componentImpl, initializing it if necessary.
func (w *weavelet) getImpl(ctx context.Context, c *component) (*componentImpl, error) {
	init := func(c *component) error {
		// We have to initialize these fields before passing to c.info.fn
		// because the user's constructor may use them.
		//
		// TODO(mwhittaker): Passing a component to a constructor while the
		// component is still being constructed is easy to get wrong. Figure out a
		// way to make this less error-prone.
		c.impl = &componentImpl{component: c}
		c.logger = slog.New(&logging.LogHandler{
			Opts: logging.Options{
				App:        w.info.App,
				Deployment: w.info.DeploymentId,
				Component:  c.info.Name,
				Weavelet:   w.info.Id,
			},
			Write: w.env.CreateLogSaver(),
		})
		c.tracer = w.tracer

		w.env.SystemLogger().Debug("Constructing component", "component", c.info.Name)
		if err := w.createComponent(ctx, c); err != nil {
			w.env.SystemLogger().Error("Constructing component failed", "err", err, "component", c.info.Name)
			return err
		}
		w.env.SystemLogger().Debug("Constructing component succeeded", "component", c.info.Name)

		c.impl.serverStub = c.info.ServerStubFn(c.impl.impl, func(key uint64, v float64) {
			if c.info.Routed {
				if err := c.load.add(key, v); err != nil {
					c.logger.Error("add load", "err", err, "component", c.info.Name, "key", key)
				}
			}
		})
		return nil
	}
	c.implInit.Do(func() { c.implErr = init(c) })
	return c.impl, c.implErr
}

func (w *weavelet) createComponent(ctx context.Context, c *component) error {
	if obj, ok := w.overrides[c.info.Iface]; ok {
		// Use supplied implementation (typically a weavertest fake).
		c.impl.impl = obj
		return nil
	}

	// Create the implementation object.
	v := reflect.New(c.info.Impl)
	obj := v.Interface()

	// Fill config if necessary.
	if cfg := config.Config(v); cfg != nil {
		// Populate the *T.
		if err := runtime.ParseConfigSection(c.info.Name, "", c.wlet.info.Sections, cfg); err != nil {
			return err
		}
	}

	// Set obj.Implements.component to c.
	if i, ok := obj.(interface{ setInstance(*componentImpl) }); !ok {
		return fmt.Errorf("component %q: type %T is not a component implementation", c.info.Name, obj)
	} else {
		i.setInstance(c.impl)
	}

	// Fill ref fields.
	err := fillRefs(obj, func(refType reflect.Type) (any, error) {
		sub, err := w.getComponentByType(refType)
		if err != nil {
			return nil, err
		}
		r, _, err := w.getInstance(ctx, sub, c.info.Name)
		return r, err
	})
	if err != nil {
		return err
	}

	// Fill listener fields.
	err = fillListeners(obj, w.getListener)
	if err != nil {
		return err
	}

	// Call Init if available.
	if i, ok := obj.(interface{ Init(context.Context) error }); ok {
		if err := i.Init(ctx); err != nil {
			return fmt.Errorf("component %q initialization failed: %w", c.info.Name, err)
		}
	}
	c.impl.impl = obj
	return nil
}

func (w *weavelet) repeatedly(errMsg string, f func() error) error {
	for r := retry.Begin(); r.Continue(w.ctx); {
		if err := f(); err != nil {
			w.env.SystemLogger().Error(errMsg+"; will retry", "err", err)
			continue
		}
		return nil
	}
	return fmt.Errorf("%s: %w", errMsg, w.ctx.Err())
}

// getClient returns a component's network client, initializing it if necessary.
func (w *weavelet) getClient(c *component) *client {
	c.clientInit.Do(func() {
		c.client = &client{
			resolver: newRoutingResolver(),
			balancer: newRoutingBalancer(c.clientTLS),
		}
	})
	return c.client
}

// getStub returns a component's componentStub, initializing it if necessary.
func (w *weavelet) getStub(c *component) (*stub, error) {
	init := func(c *component) error {
		// Initialize the client.
		w.env.SystemLogger().Debug("Creating a connection to a remote component...", "component", c.info.Name)
		client := w.getClient(c)

		// Create the client connection.
		opts := w.transport.clientOpts
		conn, err := call.Connect(w.ctx, client.resolver, opts)
		if err != nil {
			w.env.SystemLogger().Error("Creating a connection to remote component failed", "err", err, "component", c.info.Name)
			return err
		}
		if err := waitUntilReady(w.ctx, conn); err != nil {
			w.env.SystemLogger().Error("Waiting for remote component failed", "err", err, "component", c.info.Name)
			return err
		}

		w.env.SystemLogger().Debug("Creating connection to remote component succeeded", "component", c.info.Name)

		// Construct the keys for the methods.
		n := c.info.Iface.NumMethod()
		methods := make([]call.MethodKey, n)
		for i := 0; i < n; i++ {
			mname := c.info.Iface.Method(i).Name
			methods[i] = call.MakeMethodKey(c.info.Name, mname)
		}

		var balancer call.Balancer
		if c.info.Routed {
			balancer = client.balancer
		}
		c.stub = &stub{
			component: c.info.Name,
			conn:      conn,
			methods:   methods,
			balancer:  balancer,
			tracer:    w.tracer,
		}
		return nil
	}
	c.stubInit.Do(func() { c.stubErr = init(c) })
	return c.stub, c.stubErr
}

func waitUntilReady(ctx context.Context, client call.Connection) error {
	for r := retry.Begin(); r.Continue(ctx); {
		_, err := client.Call(ctx, readyMethodKey, nil, call.CallOptions{})
		if err == nil || !errors.Is(err, call.Unreachable) {
			return err
		}
	}
	return ctx.Err()
}

// startWork runs fn() in the background.
// Errors are fatal unless we have already been canceled.
func startWork(ctx context.Context, msg string, fn func() error) {
	go func() {
		if err := fn(); err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err)
			os.Exit(1)
		}
	}()
}

var _ call.Listener = &server{}

func (s *server) Accept() (net.Conn, *call.HandlerMap, error) {
	conn, err := s.Listener.Accept()
	if err != nil {
		return nil, nil, err
	}

	if !s.wlet.info.Mtls {
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
			accessibleComponents, err = s.wlet.env.VerifyClientCertificate(s.wlet.ctx, rawCerts)
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
