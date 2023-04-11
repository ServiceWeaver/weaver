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
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/net/call"
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

	root             *component                  // The automatically created "root" component
	componentsByName map[string]*component       // component name -> component
	componentsByType map[reflect.Type]*component // component type -> component

	// TODO(mwhittaker): We have one client for every component. Every client
	// independently maintains network connections to every weavelet hosting
	// the component. Thus, there may be many redundant network connections to
	// the same weavelet. Given n weavelets hosting m components, there's at
	// worst n^2m connections rather than a more optimal n^2 (a single
	// connection between every pair of weavelets). We should rewrite things to
	// avoid the redundancy.
	clientsLock sync.Mutex
	tcpClients  map[string]*client // indexed by component
}

type transport struct {
	clientOpts call.ClientOptions
	serverOpts call.ServerOptions
}

type client struct {
	client   call.Connection
	resolver *routingResolver
	balancer *routingBalancer
}

// Ensure that WeaveletHandler remains in-sync with conn.WeaveletHandler.
var _ conn.WeaveletHandler = &weavelet{}

// newWeavelet returns a new weavelet.
func newWeavelet(ctx context.Context, componentInfos []*codegen.Registration) (*weavelet, error) {
	byName := make(map[string]*component, len(componentInfos))
	byType := make(map[reflect.Type]*component, len(componentInfos))
	w := &weavelet{
		ctx:              ctx,
		componentsByName: byName,
		componentsByType: byType,
		tcpClients:       map[string]*client{},
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
			logger: slog.New(slog.HandlerOptions{Level: slog.LevelError + 1}.NewTextHandler(os.Stdout)),
		}
		byName[info.Name] = c
		byType[info.Iface] = c
	}
	main, ok := byName["main"]
	if !ok {
		return nil, fmt.Errorf("internal error: no main component registered")
	}
	main.impl = &componentImpl{component: main}

	const instrumentationLibrary = "github.com/ServiceWeaver/weaver/serviceweaver"
	const instrumentationVersion = "0.0.1"
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(env.CreateTraceExporter()),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(fmt.Sprintf("serviceweaver/%s", info.Id)),
			semconv.ProcessPIDKey.Int(os.Getpid()),
			traceio.AppNameTraceKey.String(info.App),
			traceio.VersionTraceKey.String(info.DeploymentId),
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
	main.tracer = tracer
	w.root = main

	return w, nil
}

// start starts a weavelet, executing the logic to start and manage components.
// If Start fails, it returns a non-nil error.
// Otherwise, if this process hosts "main", start returns the main component.
// Otherwise, Start never returns.
func (w *weavelet) start() (Instance, error) {
	// Launch status server for single process deployments.
	if single, ok := w.env.(*singleprocessEnv); ok {
		go func() {
			if err := single.serveStatus(w.ctx); err != nil {
				single.SystemLogger().Error("status server", "err", err)
			}
		}()
	}

	// Create handlers for all of the components served by the weavelet. Note
	// that the components themselves may not be started, but we still register
	// their handlers because we want to avoid concurrency issues with on-demand
	// handler additions.
	//
	// TODO(mwhittaker): Only add handlers for a component once it has started?
	// This will likely require us to update the set of handlers *after* we
	// start serving them, which might require some additional locking.
	handlers := &call.HandlerMap{}
	for _, c := range w.componentsByName {
		w.addHandlers(handlers, c)
	}
	// Add a dummy "ready" handler. Clients will repeatedly call this RPC until
	// it responds successfully, ensuring the server is ready.
	handlers.Set("", "ready", func(context.Context, []byte) ([]byte, error) {
		return nil, nil
	})

	if w.info.RunMain {
		// Set appropriate logger and tracer for main.
		w.root.logger = slog.New(&logging.LogHandler{
			Opts: logging.Options{
				App:        w.root.info.Name,
				Deployment: w.info.DeploymentId,
				Component:  w.root.info.Name,
				Weavelet:   w.info.Id,
			},
			Write: w.env.CreateLogSaver(),
		})
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
		addr := fmt.Sprintf("tcp://%s", lis.Addr().String())
		w.dialAddr = addr
		for _, c := range w.componentsByName {
			if c.info.Routed {
				// TODO(rgrandl): In the future, we may want to collect load for all components.
				c.load = newLoadCollector(c.info.Name, addr)
			}
		}

		startWork(w.ctx, "handle calls", func() error {
			return call.Serve(w.ctx, lis, handlers, w.transport.serverOpts)
		})
	}

	w.logRolodexCard()

	// Every Service Weaver process launches a watchComponentsToStart goroutine that
	// periodically starts components, as needed. Every Service Weaver process can host one or
	// more Service Weaver components. The components assigned to a Service Weaver process is predetermined,
	// but a component O is started lazily only when O.Get() is called by some
	// other component. So, a process may not be running all the components it has been
	// assigned. For example, process P may be assigned components X, Y, and Z but
	// only running components X and Y.
	//
	// How does a process know what components to run? Every Service Weaver process notifies
	// the runtime about the set of components that should be started. When a component
	// A in process PA calls B.Get() for a component B assigned to process PB, A
	// notifies the runtime that "B" should start. Process PB watches the set of
	// components it should start from the runtime, and starts the new components accordingly.
	//
	// Note that if a component is started locally (e.g., a component in a process
	// calls Get("B") for a component B assigned to the same process), then the
	// component's name is also registered with the protos.
	if w.info.RunMain {
		return w.root.impl, nil
	}

	// Not the main-process. Run forever.
	// TODO(mwhittaker): Catch and return errors.
	select {}
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
// is local, the returned instance is local. Otherwise, it's a network client.
// requester is the name of the requesting component.
func (w *weavelet) getInstance(c *component, requester string) (interface{}, error) {
	// Register the component.
	c.registerInit.Do(func() {
		w.env.SystemLogger().Debug("Registering component...", "component", c.info.Name)
		errMsg := fmt.Sprintf("cannot register component %q to start", c.info.Name)
		c.registerErr = w.repeatedly(errMsg, func() error {
			return w.env.ActivateComponent(w.ctx, c.info.Name, c.info.Routed)
		})
		if c.registerErr != nil {
			w.env.SystemLogger().Error("Registering component failed", "err", c.registerErr, "component", c.info.Name)
		} else {
			w.env.SystemLogger().Debug("Registering component succeeded", "component", c.info.Name)
		}
	})
	if c.registerErr != nil {
		return nil, c.registerErr
	}

	if c.local.Read() {
		impl, err := w.getImpl(c)
		if err != nil {
			return nil, err
		}
		return c.info.LocalStubFn(impl.impl, impl.component.tracer), nil
	}

	stub, err := w.getStub(c)
	if err != nil {
		return nil, err
	}
	return c.info.ClientStubFn(stub.stub, requester), nil
}

// getListener returns a network listener with the given name.
func (w *weavelet) getListener(name string, opts ListenerOptions) (*Listener, error) {
	if name == "" {
		return nil, fmt.Errorf("getListener(%q): empty listener name", name)
	}

	// Get the address to listen on.
	addr, err := w.env.GetListenerAddress(w.ctx, name, opts)
	if err != nil {
		return nil, fmt.Errorf("getListener(%q): %w", name, err)
	}

	// Listen on the address.
	l, err := net.Listen("tcp", addr.Address)
	if err != nil {
		return nil, fmt.Errorf("getListener(%q): %w", name, err)
	}

	// Export the listener.
	errMsg := fmt.Sprintf("getListener(%q): error exporting listener %v", name, l.Addr())
	var reply *protos.ExportListenerReply
	if err := w.repeatedly(errMsg, func() error {
		var err error
		reply, err = w.env.ExportListener(w.ctx, name, l.Addr().String(), opts)
		return err
	}); err != nil {
		return nil, err
	}
	if reply.Error != "" {
		return nil, fmt.Errorf("getListener(%q): %s", name, reply.Error)
	}
	return &Listener{Listener: l, proxyAddr: reply.ProxyAddress}, nil
}

// addHandlers registers a component's methods as handlers in stub.HandlerMap.
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
			impl, err := w.getImpl(c)
			if err != nil {
				return nil, err
			}
			fn := impl.serverStub.GetStubFn(mname)
			return fn(ctx, args)
		}
		handlers.Set(c.info.Name, mname, handler)
	}
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
				w.env.SystemLogger().Error("getComponent", err, "component", component)
				return
			}
			if _, err = w.getImpl(c); err != nil {
				// TODO(mwhittaker): Propagate errors.
				w.env.SystemLogger().Error("getImpl", err, "component", component)
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

	// Update resolver and balancer.
	client := w.getTCPClient(req.RoutingInfo.Component)
	endpoints, err := parseEndpoints(req.RoutingInfo.Replicas)
	if err != nil {
		return nil, err
	}
	client.resolver.update(endpoints)
	client.balancer.update(req.RoutingInfo.Assignment)

	// Update local.
	c, err := w.getComponent(req.RoutingInfo.Component)
	if err != nil {
		return nil, err
	}
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
func (w *weavelet) getImpl(c *component) (*componentImpl, error) {
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
		if err := createComponent(w.ctx, c); err != nil {
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

func createComponent(ctx context.Context, c *component) error {
	// Create the implementation object.
	obj := c.info.New()

	if c.info.ConfigFn != nil {
		cfg := c.info.ConfigFn(obj)
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

// getStub returns a component's componentStub, initializing it if necessary.
func (w *weavelet) getStub(c *component) (*componentStub, error) {
	init := func(c *component) error {
		// Initialize the client.
		w.env.SystemLogger().Debug("Getting TCP client to component...", "component", c.info.Name)
		client := w.getTCPClient(c.info.Name)
		if err := client.init(w.ctx, w.transport.clientOpts); err != nil {
			w.env.SystemLogger().Error("Getting TCP client to component failed", "err", err, "component", c.info.Name)
			return err
		}
		w.env.SystemLogger().Debug("Getting TCP client to component succeeded", "component", c.info.Name)

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
		c.stub = &componentStub{
			stub: &stub{
				client:   client.client,
				methods:  methods,
				balancer: balancer,
				tracer:   w.tracer,
			},
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

// getTCPClient returns the TCP client for the provided component.
func (w *weavelet) getTCPClient(component string) *client {
	// Create entry in client map.
	w.clientsLock.Lock()
	defer w.clientsLock.Unlock()
	c, ok := w.tcpClients[component]
	if !ok {
		c = &client{
			resolver: newRoutingResolver(),
			balancer: newRoutingBalancer(),
		}
		w.tcpClients[component] = c
	}
	return c
}

// init initializes a client. It should only be called once.
func (c *client) init(ctx context.Context, opts call.ClientOptions) error {
	var err error
	c.client, err = call.Connect(ctx, c.resolver, opts)
	if err != nil {
		return err
	}
	return waitUntilReady(ctx, c.client)
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
