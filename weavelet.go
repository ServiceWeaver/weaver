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
)

// readyMethodKey holds the key for a method used to check if a backend is ready.
var readyMethodKey = call.MakeMethodKey("", "ready")

// WeaveletHandler implements the weavelet side processing of messages exchanged
// with the corresponding envelope.
type WeaveletHandler interface {
	// CollectLoad returns the latest load information at the weavelet.
	CollectLoad() (*protos.WeaveletLoadReport, error)
}

// A weavelet is responsible for running and managing Service Weaver components. As the
// name suggests, a weavelet is analogous to a kubelet or borglet. Every
// weavelet executes components for a single Service Weaver process, but a process may be
// implemented by multiple weavelets.
type weavelet struct {
	ctx       context.Context
	env       env                  // Manages interactions with execution environment
	info      *protos.WeaveletInfo // Information about this weavelet
	transport *transport           // Transport for cross-group communication
	dialAddr  call.NetworkAddress  // Address this weavelet is reachable at
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

	loads map[string]*loadCollector // load for every local routed component
}

type transport struct {
	clientOpts call.ClientOptions
	serverOpts call.ServerOptions
}

type client struct {
	init     sync.Once
	client   call.Connection
	routelet *routelet
	err      error
}

// Ensure that WeaveletHandler remains in-sync with conn.WeaveletHandler.
var (
	_ WeaveletHandler      = conn.WeaveletHandler(nil)
	_ conn.WeaveletHandler = WeaveletHandler(nil)
	_ WeaveletHandler      = &weavelet{}
)

// newWeavelet returns a new weavelet.
func newWeavelet(ctx context.Context, componentInfos []*codegen.Registration) (*weavelet, error) {
	byName := make(map[string]*component, len(componentInfos))
	byType := make(map[reflect.Type]*component, len(componentInfos))
	w := &weavelet{
		ctx:              ctx,
		componentsByName: byName,
		componentsByType: byType,
		tcpClients:       map[string]*client{},
		loads:            map[string]*loadCollector{},
	}
	env, err := getEnv(ctx, w)
	if err != nil {
		return nil, err
	}
	w.env = env

	wletInfo := env.GetWeaveletInfo()
	if wletInfo == nil {
		return nil, fmt.Errorf("unable to get weavelet information")
	}
	w.info = wletInfo

	exporter, err := env.CreateTraceExporter()
	if err != nil {
		return nil, fmt.Errorf("internal error: cannot create trace exporter: %w", err)
	}

	for _, info := range componentInfos {
		c := &component{
			wlet: w,
			info: info,
			// may be remote, so start with no-op logger. May set real logger later.
			logger: discardingLogger{},
		}
		byName[info.Name] = c
		byType[info.Iface] = c
	}
	main, ok := byName["main"]
	if !ok {
		return nil, fmt.Errorf("internal error: no main component registered")
	}
	main.impl = &componentImpl{component: main}

	// Place components into colocation groups and OS processes.
	if err := place(byName, wletInfo); err != nil {
		return nil, err
	}

	const instrumentationLibrary = "github.com/ServiceWeaver/weaver/serviceweaver"
	const instrumentationVersion = "0.0.1"
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(fmt.Sprintf("serviceweaver/%s/%s", wletInfo.Group, wletInfo.Id[:4])),
			semconv.ProcessPIDKey.Int(os.Getpid()),
			traceio.AppNameTraceKey.String(wletInfo.App),
			traceio.VersionTraceKey.String(wletInfo.DeploymentId),
			traceio.ColocationGroupNameTraceKey.String(wletInfo.Group.Name),
			traceio.GroupReplicaIDTraceKey.String(wletInfo.GroupId),
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
				single.SystemLogger().Error("status server", err)
			}
		}()
	}

	// Create handlers for all of the components served by the weavelet. Note
	// that the components themselves may not be started, but we still register
	// their handlers because we want to avoid concurrency issues with on-demand
	// handler additions.
	handlers := &call.HandlerMap{}
	for _, c := range w.componentsByName {
		if w.inLocalGroup(c) {
			w.addHandlers(handlers, c)
		}
	}
	// Add a dummy "ready" handler. Clients will repeatedly call this RPC until
	// it responds successfully, ensuring the server is ready.
	handlers.Set("", "ready", func(context.Context, []byte) ([]byte, error) {
		return nil, nil
	})

	isMain := w.info.Group.Name == "main"
	if isMain {
		// Set appropriate logger and tracer for main.
		logSaver := w.env.CreateLogSaver(w.ctx, "main")
		w.root.logger = newAttrLogger(
			w.root.info.Name, w.info.DeploymentId, w.root.info.Name, w.info.Id, logSaver)
	}

	// For a singleprocess deployment, no server is launched because all
	// method invocations are process-local and executed as regular go function
	// calls.
	if !w.info.SingleProcess {
		// TODO(mwhittaker): Right now, we resolve our hostname to get a
		// dialable IP address. Double check that this always works.
		host := "localhost"
		if !w.info.SingleMachine {
			var err error
			host, err = os.Hostname()
			if err != nil {
				return nil, fmt.Errorf("error getting local hostname: %w", err)
			}
		}
		lis, dialAddr, err := w.listen("tcp", fmt.Sprintf("%s:0", host))
		if err != nil {
			return nil, fmt.Errorf("error creating internal listener: %w", err)
		}
		w.dialAddr = dialAddr

		for _, c := range w.componentsByName {
			if w.inLocalGroup(c) && c.info.Routed {
				// TODO(rgrandl): In the future, we may want to collect load for all components.
				w.loads[c.info.Name] = newLoadCollector(c.info.Name, dialAddr)
			}
		}

		// Monitor our routing assignment.
		routelet := newRoutelet(w.ctx, w.env, w.info.Group.Name)
		routelet.onChange(w.onNewRoutingInfo)

		// Register our external address. This will allow weavelets in other
		// colocation groups to detect this weavelet and load-balance traffic
		// to it. The internal address needs not be registered since it is
		// not replicated and the caller is able to infer it.
		//
		// TODO(mwhittaker): Remove our address from the store if we crash. If
		// we exit gracefully, we can do this easily. If the machine on which
		// this weavelet is running straight up crashes, then that's a bit more
		// challenging. We may have to have TTLs in the store, or maybe have a
		// nanny monitor for failures.
		const errMsg = "cannot register weavelet replica"
		if err := w.repeatedly(errMsg, func() error {
			return w.env.RegisterReplica(w.ctx, dialAddr)
		}); err != nil {
			return nil, err
		}

		serve := func(lis net.Listener, transport *transport) {
			if lis == nil || transport == nil {
				return
			}

			// Arrange to close the listener when we are canceled.
			go func() {
				<-w.ctx.Done()
				lis.Close()
			}()

			// Start serving the transport. This should be done prior to calling
			// Get() below, since:
			//  1. Get() may block waiting for the remote process to begin serving,
			//     which can cause unnecessary serving delays for this process, and
			//  2. Get() may assign a random unused port to the component, which may
			//     conflict with the port used by the transport.
			startWork(w.ctx, "handle calls", func() error {
				return call.Serve(w.ctx, lis, handlers, transport.serverOpts)
			})
		}
		serve(lis, w.transport)
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
	if isMain {
		// Watch for components in a goroutine since this goroutine will return to user main.
		startWork(w.ctx, "watch for components to start", w.watchComponentsToStart)
		return w.root.impl, nil
	}

	// Not the main-process. Run forever, or until there is an error.
	return nil, w.watchComponentsToStart()
}

// logRolodexCard pretty prints a card that includes basic information about
// the weavelet. It looks something like this:
//
//	┌ weavelet 5b2d9d03-d21e-4ae9-a875-eab80af85350 started ┐
//	│   hostname   : alan.turing.com                        │
//	│   deployment : f20bbe05-85a5-4596-bab6-60e75b366306   │
//	│   group      : cache.IntCache                         │
//	│   group id   : 0da893cd-ba9a-47e4-909f-8d5faa924275   │
//	│   components : [cache.IntCache cache.StringCache]     │
//	│   address:   : tcp://127.0.0.1:43937                  │
//	│   pid        : 836347                                 │
//	└───────────────────────────────────────────────────────┘
func (w *weavelet) logRolodexCard() {
	var localComponents []string
	for name, c := range w.componentsByName {
		if w.inLocalGroup(c) {
			localComponents = append(localComponents, logging.ShortenComponent(name))
		}
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "UNKNOWN"
	}

	header := fmt.Sprintf(" weavelet %s started ", w.info.Id)
	lines := []string{
		fmt.Sprintf("   hostname   : %s ", hostname),
		fmt.Sprintf("   deployment : %s ", w.info.DeploymentId),
		fmt.Sprintf("   group      : %s ", logging.ShortenComponent(w.info.Group.Name)),
		fmt.Sprintf("   group id   : %s ", w.info.GroupId),
		fmt.Sprintf("   components : %v ", localComponents),
		fmt.Sprintf("   address    : %s", string(w.dialAddr)),
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
	// Consider the scenario where component A invokes a method on component B.
	// If we're running as a single process, all communication is local.
	// Otherwise, here's a table showing which type of communication we use.
	//
	//                                    B is...
	//                               unrouted routed
	//             same coloc group | local  | tcp  |
	//             diff coloc group | tcp    | tcp  |
	//                              +--------+------+
	//
	// Note that if B is routed, we don't use local communication, even if B
	// is in the same colocation group as A. The reason is that A's call may
	// get routed to an instance of B in a different colocation group.
	var local bool // should we perform local, in-process communication?
	switch {
	case w.info.SingleProcess:
		local = true
	case c.info.Routed:
		local = false
	default:
		local = w.inLocalGroup(c)
	}

	if local {
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
	addr, err := w.env.GetAddress(w.ctx, name, opts)
	if err != nil {
		return nil, fmt.Errorf("getListener(%q): %w", name, err)
	}

	// Listen on the address.
	l, err := net.Listen("tcp", addr.Address)
	if err != nil {
		return nil, fmt.Errorf("getListener(%q): %w", name, err)
	}

	// Export the listener.
	lis := &protos.Listener{Name: name, Addr: l.Addr().String()}
	errMsg := fmt.Sprintf("getListener(%q): error exporting listener %v", name, lis.Addr)
	var reply *protos.ExportListenerReply
	if err := w.repeatedly(errMsg, func() error {
		var err error
		reply, err = w.env.ExportListener(w.ctx, lis, opts)
		return err
	}); err != nil {
		return nil, err
	}
	if reply.Error != "" {
		return nil, fmt.Errorf("getListener(%q): %s", name, reply.Error)
	}
	return &Listener{Listener: l, proxyAddr: reply.ProxyAddress}, nil
}

// inLocalGroup returns true iff the component is hosted in the same colocation
// group as us.
func (w *weavelet) inLocalGroup(c *component) bool {
	return c.groupName == w.info.Group.Name
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

// watchComponentsToStart is a long running goroutine that is responsible for
// starting components.
func (w *weavelet) watchComponentsToStart() error {
	var version *call.Version

	for r := retry.Begin(); r.Continue(w.ctx); {
		componentsToStart, newVersion, err := w.env.GetComponentsToStart(w.ctx, version)
		if err != nil {
			w.env.SystemLogger().Error("cannot get components to start; will retry", err)
			continue
		}
		version = newVersion
		for _, start := range componentsToStart {
			// TODO(mwhittaker): Start a component on a separate goroutine?
			// Right now, if one component hangs forever in its constructor, no
			// other components can start. main actually does this intentionally.
			c, err := w.getComponent(start)
			if err != nil {
				return err
			}
			if _, err = w.getImpl(c); err != nil {
				return err
			}
		}
		r.Reset()
	}
	return w.ctx.Err()
}

// CollectLoad implements the WeaveletHandler interface.
func (w *weavelet) CollectLoad() (*protos.WeaveletLoadReport, error) {
	report := &protos.WeaveletLoadReport{
		App:          w.info.App,
		DeploymentId: w.info.DeploymentId,
		Group:        w.info.Group.Name,
		Replica:      string(w.dialAddr),
		Loads:        map[string]*protos.WeaveletLoadReport_ComponentLoad{},
	}

	for c, collector := range w.loads {
		if x := collector.report(); x != nil {
			report.Loads[c] = x
		}
		// TODO(mwhittaker): If ReportLoad down below fails, we
		// likely don't want to reset our load.
		collector.reset()
	}
	return report, nil
}

// onNewRoutingInfo is a callback that is invoked every time the routing info
// for our process changes. onNewRoutingInfo updates the assignments and load
// for our local components.
func (w *weavelet) onNewRoutingInfo(info *protos.RoutingInfo) {
	if info.Assignment == nil {
		return
	}
	collector, ok := w.loads[info.Assignment.Component]
	if !ok {
		return
	}
	collector.updateAssignment(info.Assignment)
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
	if !w.inLocalGroup(c) {
		return nil, fmt.Errorf("component %q is not local", c.info.Name)
	}

	init := func(c *component) error {
		if err := w.env.RegisterComponentToStart(w.ctx, w.info.Group.Name, c.info.Name, c.info.Routed); err != nil {
			return fmt.Errorf("component %q registration failed: %w", c.info.Name, err)
		}

		// We have to initialize these fields before passing to c.info.fn
		// because the user's constructor may use them.
		//
		// TODO(mwhittaker): Passing a component to a constructor while the
		// component is still being constructed is easy to get wrong. Figure out a
		// way to make this less error-prone.
		c.impl = &componentImpl{component: c}
		logSaver := w.env.CreateLogSaver(w.ctx, c.info.Name)
		logger := newAttrLogger(w.info.App, w.info.DeploymentId, c.info.Name, w.info.Id, logSaver)
		c.logger = logger
		c.tracer = w.tracer

		w.env.SystemLogger().Debug("Constructing component", "component", c.info.Name)
		if err := createComponent(w.ctx, c); err != nil {
			return err
		}

		c.impl.serverStub = c.info.ServerStubFn(c.impl.impl, func(key uint64, v float64) {
			if c.info.Routed {
				if err := w.loads[c.info.Name].add(key, v); err != nil {
					logger.Error("add load", err, "component", c.info.Name, "key", key)
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
			w.env.SystemLogger().Error(errMsg+"; will retry", err)
			continue
		}
		return nil
	}
	return fmt.Errorf("%s: %w", errMsg, w.ctx.Err())
}

// getStub returns a component's componentStub, initializing it if necessary.
func (w *weavelet) getStub(c *component) (*componentStub, error) {
	init := func(c *component) error {
		// Register the component's name to start. The remote watcher will notice
		// the name and launch the component.
		errMsg := fmt.Sprintf("cannot register component %q to start", c.info.Name)
		if err := w.repeatedly(errMsg, func() error {
			return w.env.RegisterComponentToStart(w.ctx, c.groupName, c.info.Name, c.info.Routed)
		}); err != nil {
			return err
		}

		client, err := w.getTCPClient(c)
		if err != nil {
			return err
		}

		// Construct the keys for the methods.
		n := c.info.Iface.NumMethod()
		methods := make([]call.MethodKey, n)
		for i := 0; i < n; i++ {
			mname := c.info.Iface.Method(i).Name
			methods[i] = call.MakeMethodKey(c.info.Name, mname)
		}

		var balancer call.Balancer
		if c.info.Routed {
			balancer = client.routelet.balancer()
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

func (w *weavelet) getTCPClient(component *component) (*client, error) {
	// Create entry in client map.
	w.clientsLock.Lock()
	c, ok := w.tcpClients[component.info.Name]
	if !ok {
		c = &client{}
		w.tcpClients[component.groupName] = c
	}
	w.clientsLock.Unlock()

	// Initialize (or wait for initialization to complete.)
	c.init.Do(func() {
		routelet := newRoutelet(w.ctx, w.env, component.info.Name)
		c.routelet = routelet
		c.client, c.err = call.Connect(w.ctx, routelet.resolver(), w.transport.clientOpts)
		if c.err != nil {
			return
		}
		c.err = waitUntilReady(w.ctx, c.client)
		if c.err != nil {
			c.client = nil
			return
		}
	})
	return c, c.err
}

// place places registered components into colocation groups.
//
// The current placement approach:
//   - If a group of components appear in the 'same_process' entry in the
//     Service Weaver application config, they are placed in the same
//     colocation group.
//   - Every other component is placed in a colocation group of its own.
//
// TODO(spetrovic): Consult envelope for placement.
func place(registry map[string]*component, w *protos.WeaveletInfo) error {
	if w.SingleProcess {
		// If we are running a singleprocess deployment, all the components are
		// assigned to the same colocation group.
		for _, c := range registry {
			c.groupName = "main"
		}
		return nil
	}

	// NOTE: the config should already have been checked to ensure that each
	// component appears at most once in the same_process entry.
	for _, components := range w.SameProcess {
		// Verify components and assign the (same) colocation group name for
		// them.
		var groupName string
		for _, component := range components.Components {
			c := registry[component]
			if c == nil {
				return fmt.Errorf("component %q not registered", component)
			}
			if groupName == "" {
				groupName = c.info.Name
			}
			c.groupName = groupName
		}
	}

	// Assign every unplaced component to a group of its own.
	for _, c := range registry {
		if c.groupName == "" {
			c.groupName = c.info.Name
		}
	}

	return nil
}

// listen returns a network listener for the given listening address and
// network, along with a dialable address that can be used to reach
// the listener.
func (w *weavelet) listen(network, address string) (net.Listener, call.NetworkAddress, error) {
	var lis net.Listener
	var err error
	switch network {
	case "unix", "tcp":
		if network == "unix" && len(address) > 108 {
			// For unix sockets, the maximum length is 108 chars.
			// http://shortn/_eRBZrR3ICt
			return nil, "", fmt.Errorf("maximum length for unix-sockets is 108 chars, got %d: %q", len(address), address)
		}
		lconfig := &net.ListenConfig{}
		lis, err = lconfig.Listen(w.ctx, network, address)
	default:
		return nil, "", fmt.Errorf("unsupported network protocol %q", network)
	}
	if err != nil {
		return nil, "", err
	}
	dialAddr := call.NetworkAddress(fmt.Sprintf("%s://%s", lis.Addr().Network(), lis.Addr().String()))
	return lis, dialAddr, nil
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
