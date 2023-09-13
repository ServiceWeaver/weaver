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

package sim

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	core "github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/weaver"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"golang.org/x/exp/maps"
	"golang.org/x/sync/errgroup"
)

// options configures a simulator.
type options struct {
	Seed        int64   // the simulator's seed
	NumReplicas int     // the number of replicas of every component
	NumOps      int     // the number of ops to run
	FailureRate float64 // the fraction of calls to artificially fail
	YieldRate   float64 // the probability that an op yields after a step
}

// simulator deterministically simulates a Service Weaver application.
type simulator struct {
	name       string                                 // test name
	workload   reflect.Value                          // workload instance
	regsByIntf map[reflect.Type]*codegen.Registration // regs, by component interface
	info       componentInfo                          // component metadata
	config     *protos.AppConfig                      // application config
	opts       options                                // simulator options
	components map[string][]any                       // component replicas
	ops        []*op                                  // registered ops

	ctx   context.Context // simulation context
	group *errgroup.Group // group with all running goroutines

	mu          sync.Mutex       // guards the following fields
	rand        *rand.Rand       // random number generator
	current     int              // currently running op
	numStarted  int              // number of started ops
	notFinished ints             // not finished op trace ids, optimized for removal and sampling
	calls       map[int][]*call  // pending calls, by trace id
	replies     map[int][]*reply // pending replies, by trace id
	history     []Event          // history of events
	nextTraceID int              // next trace id
	nextSpanID  int              // next span id
}

// generator is an untyped Generator[T].
type generator func(*rand.Rand) reflect.Value

// An op is a randomized operation performed as part of a simulation. ops are
// derived from exported methods on a workload struct.
type op struct {
	t          reflect.Type  // method type
	name       string        // the name of the op
	arity      int           // arity of op, ignoring context.Context argument
	generators []generator   // the op's generators
	f          reflect.Value // the op method itself
}

// fate dictates if and how a call should fail.
type fate int

const (
	// Don't fail the call.
	dontFail fate = iota

	// Fail the call before it is delivered to a component replica. In this
	// case, the method call does not execute at all.
	failBeforeDelivery

	// Fail the call after it has been delivered to a component replica. In
	// this case, the method call will fail after fully executing.
	//
	// TODO(mwhittaker): Deliver a method call, and then fail it while it is
	// still executing.
	failAfterDelivery
)

// call is a pending method call.
type call struct {
	traceID   int
	spanID    int
	fate      fate            // whether to fail the operation
	component reflect.Type    // the component being called
	method    string          // the method being called
	args      []reflect.Value // the call's arguments
	reply     chan *reply     // a channel to receive the call's reply
}

// reply is a pending method reply.
type reply struct {
	call    *call           // the corresponding call
	returns []reflect.Value // the call's return values
}

// TODO(mwhittaker): If a user doesn't propagate contexts correctly, we lose
// trace and span ids. Detect this and return an error.

// We store trace and span ids in the context using the following key.
// According to the Go spec, "pointers to distinct zero-size variables may or
// may not be equal" [1]. Thus, to make sure that traceContextKey is unique, we
// make sure it is not a pointer to a zero-sized variable.
//
// [1]: https://go.dev/ref/spec#Comparison_operators
var traceContextKey = &struct{ int }{}

// A traceContext stores a trace and span ID.
type traceContext struct {
	traceID int
	spanID  int
}

// withIDs returns a context embedded with the provided trace and span id.
func withIDs(ctx context.Context, traceID, spanID int) context.Context {
	return context.WithValue(ctx, traceContextKey, traceContext{traceID, spanID})
}

// extractIDs returns the trace and span id embedded in the provided context.
// If the provided context does not have embedded trace and span ids,
// extractIDs returns 0, 0.
func extractIDs(ctx context.Context) (int, int) {
	var traceID, spanID int
	if x := ctx.Value(traceContextKey); x != nil {
		traceID = x.(traceContext).traceID
		spanID = x.(traceContext).spanID
	}
	return traceID, spanID
}

// componentInfo includes information about components.
type componentInfo struct {
	hasRefs      map[reflect.Type]bool
	hasListeners map[reflect.Type]bool
	hasConfig    map[reflect.Type]bool
}

// newSimulator returns a new simulator.
func newSimulator(name string, regsByIntf map[reflect.Type]*codegen.Registration, info componentInfo, app *protos.AppConfig) *simulator {
	s := &simulator{
		name:       name,
		config:     app,
		regsByIntf: regsByIntf,
		info:       info,
		components: make(map[string][]any, len(regsByIntf)),
		calls:      map[int][]*call{},
		replies:    map[int][]*reply{},
	}
	return s
}

func (s *simulator) reset(workload any, fakes map[reflect.Type]any, ops []*op, opts options) error {
	// Validate options.
	if opts.NumReplicas <= 0 {
		return fmt.Errorf("NumReplicas (%d) <= 0", opts.NumReplicas)
	}
	if opts.NumOps <= 0 {
		return fmt.Errorf("NumOps (%d) <= 0", opts.NumOps)
	}
	if opts.FailureRate < 0 || opts.FailureRate > 1 {
		return fmt.Errorf("FailureRate (%f) out of range [0, 1]", opts.FailureRate)
	}
	if opts.YieldRate < 0 || opts.YieldRate > 1 {
		return fmt.Errorf("YieldRate (%f) out of range [0, 1]", opts.YieldRate)
	}

	s.workload = reflect.ValueOf(workload)
	s.opts = opts
	s.ops = ops
	s.rand = rand.New(rand.NewSource(opts.Seed))
	s.current = 1
	s.numStarted = 0
	s.notFinished.reset(1, 1+opts.NumOps)
	for k, v := range s.calls {
		s.calls[k] = v[:0]
	}
	for k, v := range s.replies {
		s.replies[k] = v[:0]
	}
	s.history = []Event{}
	s.nextTraceID = 1
	s.nextSpanID = 1

	// Fill ref fields inside the workload struct.
	if err := weaver.FillRefs(workload, func(t reflect.Type) (any, error) {
		return s.getIntf(t, "op", 0)
	}); err != nil {
		return err
	}

	// Create component replicas.
	for _, reg := range s.regsByIntf {
		components := s.components[reg.Name]
		if components != nil {
			components = components[:0]
		}

		if fake, ok := fakes[reg.Iface]; ok {
			s.components[reg.Name] = append(components, fake)
			continue
		}

		for i := 0; i < opts.NumReplicas; i++ {
			// Create the component implementation.
			v := reflect.New(reg.Impl)
			obj := v.Interface()

			// Fill config.
			if s.info.hasConfig[reg.Iface] {
				if cfg := weaver.GetConfig(obj); cfg != nil {
					if err := runtime.ParseConfigSection(reg.Name, "", s.config.Sections, cfg); err != nil {
						return err
					}
				}
			}

			// Set logger.
			//
			// TODO(mwhittaker): Use custom logger.
			if err := weaver.SetLogger(obj, slog.Default()); err != nil {
				return err
			}

			// Fill ref fields.
			if s.info.hasRefs[reg.Iface] {
				if err := weaver.FillRefs(obj, func(t reflect.Type) (any, error) {
					return s.getIntf(t, reg.Name, i)
				}); err != nil {
					return err
				}
			}

			// Fill listener fields.
			if s.info.hasListeners[reg.Iface] {
				if err := weaver.FillListeners(obj, func(name string) (net.Listener, string, error) {
					lis, err := net.Listen("tcp", ":0")
					return lis, "", err
				}); err != nil {
					return err
				}
			}

			// Call Init if available.
			if i, ok := obj.(interface{ Init(context.Context) error }); ok {
				// TODO(mwhittaker): Use better context.
				if err := i.Init(context.Background()); err != nil {
					return fmt.Errorf("component %q initialization failed: %w", reg.Name, err)
				}
			}

			components = append(components, obj)
		}
		s.components[reg.Name] = components
	}

	return nil
}

// getIntf returns a handle to the component of the provided type.
func (s *simulator) getIntf(t reflect.Type, caller string, replica int) (any, error) {
	reg, ok := s.regsByIntf[t]
	if !ok {
		return nil, fmt.Errorf("component %v not found", t)
	}
	call := func(method string, ctx context.Context, args []any, returns []any) error {
		return s.call(caller, replica, reg, method, ctx, args, returns)
	}
	return reg.ReflectStubFn(call), nil
}

// call executes a component method call against a random replica.
func (s *simulator) call(caller string, replica int, reg *codegen.Registration, method string, ctx context.Context, args []any, returns []any) error {
	// Convert the arguments to reflect.Values.
	in := make([]reflect.Value, 1+len(args))
	in[0] = reflect.ValueOf(ctx)
	strings := make([]string, len(args))
	for i, arg := range args {
		in[i+1] = reflect.ValueOf(arg)
		strings[i] = fmt.Sprint(arg)
	}

	// Record the call.
	reply := make(chan *reply, 1)
	s.mu.Lock()
	traceID, _ := extractIDs(ctx)
	spanID := s.nextSpanID
	s.nextSpanID++

	// Determine the fate of the call.
	fate := dontFail
	if flip(s.rand, s.opts.FailureRate) {
		// TODO(mwhittaker): Have two parameters to control the rate of failing
		// before and after delivery? This level of control might be
		// unnecessary. For now, we pick between them equiprobably.
		if flip(s.rand, 0.5) {
			fate = failBeforeDelivery
		} else {
			fate = failAfterDelivery
		}
	}

	s.calls[traceID] = append(s.calls[traceID], &call{
		traceID:   traceID,
		spanID:    spanID,
		fate:      fate,
		component: reg.Iface,
		method:    method,
		args:      in,
		reply:     reply,
	})

	if caller == "op" {
		replica = traceID
	}
	s.history = append(s.history, Call{
		TraceID:   traceID,
		SpanID:    spanID,
		Caller:    caller,
		Replica:   replica,
		Component: reg.Name,
		Method:    method,
		Args:      strings,
	})
	s.mu.Unlock()

	// Take a step and wait for the call to finish.
	s.step()
	var out []reflect.Value
	select {
	case r := <-reply:
		out = r.returns
	case <-s.ctx.Done():
		return s.ctx.Err()
	}

	// Populate return values.
	if len(returns) != len(out)-1 {
		panic(fmt.Errorf("invalid number of returns: want %d, got %d", len(out)-1, len(returns)))
	}
	for i := 0; i < len(returns); i++ {
		// Note that returns[i] has static type any but dynamic type *T for
		// some T. out[i] has dynamic type T.
		reflect.ValueOf(returns[i]).Elem().Set(out[i])
	}
	if x := out[len(out)-1].Interface(); x != nil {
		return x.(error)
	}
	return nil
}

// Simulate executes a single simulation. Simulate returns an error if the
// simulation fails to execute properly. If the simulation executes properly
// and successfuly finds an invariant violation, no error is returned, but the
// invariant violation is reported as an error in the returned Results.
func (s *simulator) Simulate(ctx context.Context) (Results, error) {
	// TODO(mwhittaker): Catch panics.
	s.group, s.ctx = errgroup.WithContext(ctx)
	s.step()
	err := s.group.Wait()
	if err != nil {
		entry := graveyardEntry{
			Version:     version,
			Seed:        s.opts.Seed,
			NumReplicas: s.opts.NumReplicas,
			NumOps:      s.opts.NumOps,
			FailureRate: s.opts.FailureRate,
			YieldRate:   s.opts.YieldRate,
		}
		// TODO(mwhittaker): Escape names.
		dir := filepath.Join("testdata", "sim", s.name)
		writeGraveyardEntry(dir, entry)
	}
	if err != nil && err == ctx.Err() {
		return Results{}, err
	}
	return Results{Err: err, History: s.history}, nil
}

// step performs one step of a simulation.
func (s *simulator) step() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ctx.Err() != nil {
		// The simulation has been cancelled.
		return
	}

	if s.notFinished.size() == 0 {
		// The simulation is finished.
		return
	}

	if !s.notFinished.has(s.current) || flip(s.rand, s.opts.YieldRate) {
		// Yield execution to a (potentially) different op.
		s.current = s.notFinished.pick(s.rand)
	}

	if s.current > s.numStarted {
		// Make sure to start ops in increasing order. Op 1 starts first, then
		// Op 2, and so on.
		s.current = s.numStarted + 1
		s.numStarted++

		// Start the op.
		o := pick(s.rand, s.ops)
		s.group.Go(func() error {
			return s.runOp(s.ctx, o)
		})
		return
	}

	if len(s.calls[s.current]) == 0 && len(s.replies[s.current]) == 0 {
		// This should be impossible. If it ever happens, there's a bug.
		panic(fmt.Errorf("op %d has no pending calls or replies", s.current))
	}

	hasCalls := len(s.calls[s.current]) > 0
	hasReplies := len(s.replies[s.current]) > 0
	deliverCall := false
	switch {
	case hasCalls && hasReplies:
		deliverCall = flip(s.rand, 0.5)
	case hasCalls && !hasReplies:
		deliverCall = true
	case !hasCalls && hasReplies:
		deliverCall = false
	case !hasCalls && !hasReplies:
		return
	}

	// Randomly execute a step.
	if deliverCall {
		var call *call
		call, s.calls[s.current] = pop(s.rand, s.calls[s.current])

		if call.fate == failBeforeDelivery {
			// Fail the call before delivering it.
			s.history = append(s.history, DeliverError{
				TraceID: call.traceID,
				SpanID:  call.spanID,
			})
			call.reply <- &reply{
				call:    call,
				returns: returnError(call.component, call.method, core.RemoteCallError),
			}
			close(call.reply)
			return
		}

		// Deliver the call.
		s.group.Go(func() error {
			s.deliverCall(call)
			return nil
		})
	} else {
		var reply *reply
		reply, s.replies[s.current] = pop(s.rand, s.replies[s.current])

		if reply.call.fate == failAfterDelivery {
			// Fail the call after delivering it.
			s.history = append(s.history, DeliverError{
				TraceID: reply.call.traceID,
				SpanID:  reply.call.spanID,
			})
			reply.returns = returnError(reply.call.component, reply.call.method, core.RemoteCallError)
			reply.call.reply <- reply
			close(reply.call.reply)
			return
		}

		// Return successfully.
		s.history = append(s.history, DeliverReturn{
			TraceID: reply.call.traceID,
			SpanID:  reply.call.spanID,
		})
		reply.call.reply <- reply
		close(reply.call.reply)
	}
}

// runOp runs the provided operation.
func (s *simulator) runOp(ctx context.Context, o *op) error {
	s.mu.Lock()
	traceID, spanID := s.nextTraceID, s.nextSpanID
	s.nextTraceID++
	s.nextSpanID++

	// Generate random op inputs. Lock s.mu because s.rand is not safe for
	// concurrent use by multiple goroutines.
	args := make([]reflect.Value, 2+len(o.generators))
	formatted := make([]string, len(o.generators))
	args[0] = s.workload
	args[1] = reflect.ValueOf(withIDs(ctx, traceID, spanID))
	for i, generator := range o.generators {
		x := generator(s.rand)
		args[i+2] = x
		formatted[i] = fmt.Sprint(x.Interface())
	}

	// Record an OpStart event.
	s.history = append(s.history, OpStart{
		TraceID: traceID,
		SpanID:  spanID,
		Name:    o.name,
		Args:    formatted,
	})
	s.mu.Unlock()

	// Invoke the op.
	var err error
	if x := o.f.Call(args)[0].Interface(); x != nil {
		err = x.(error)
	}

	// Record an OpFinish event.
	msg := "<nil>"
	if err != nil {
		msg = err.Error()
	}
	s.mu.Lock()
	s.history = append(s.history, OpFinish{
		TraceID: traceID,
		SpanID:  spanID,
		Error:   msg,
	})
	s.notFinished.remove(traceID)
	s.mu.Unlock()

	if err != nil {
		// If an op returns a non-nil error, abort the simulation and don't
		// take another step. Because the runOp function is running inside an
		// errgroup.Group, the returned error will cancel all goroutines.
		return err
	}

	// If the op succeeded, take another step.
	s.step()
	return nil
}

// deliverCall delivers the provided pending method call.
func (s *simulator) deliverCall(call *call) {
	reg, ok := s.regsByIntf[call.component]
	if !ok {
		panic(fmt.Errorf("component %v not found", call.component))
	}

	// Pick a replica to execute the call.
	s.mu.Lock()
	index := s.rand.Intn(len(s.components[reg.Name]))
	replica := s.components[reg.Name][index]

	// Record a DeliverCall event.
	s.history = append(s.history, DeliverCall{
		TraceID:   call.traceID,
		SpanID:    call.spanID,
		Component: reg.Name,
		Replica:   index,
	})
	s.mu.Unlock()

	// Call the component method.
	returns := reflect.ValueOf(replica).MethodByName(call.method).Call(call.args)
	strings := make([]string, len(returns))
	for i, ret := range returns {
		strings[i] = fmt.Sprint(ret.Interface())
	}

	// Record the reply and take a step.
	s.mu.Lock()
	s.replies[call.traceID] = append(s.replies[call.traceID], &reply{
		call:    call,
		returns: returns,
	})

	s.history = append(s.history, Return{
		TraceID:   call.traceID,
		SpanID:    call.spanID,
		Component: reg.Name,
		Replica:   index,
		Returns:   strings,
	})
	s.mu.Unlock()
	s.step()
}

// returnError returns a slice of reflect.Values compatible with the return
// type of the provided method. The final return value is the provided error.
// All other return values are zero initialized.
func returnError(component reflect.Type, method string, err error) []reflect.Value {
	m, ok := component.MethodByName(method)
	if !ok {
		panic(fmt.Errorf("method %s.%s not found", component, method))
	}
	t := m.Type
	n := t.NumOut()
	returns := make([]reflect.Value, n)
	for i := 0; i < n-1; i++ {
		returns[i] = reflect.Zero(t.Out(i))
	}
	returns[n-1] = reflect.ValueOf(err)
	return returns
}

// Mermaid returns a [mermaid][1] diagram that illustrates a simulation
// history.
//
// TODO(mwhittaker): Arrange replicas in topological order.
//
// [1]: https://mermaid.js.org/
func (r *Results) Mermaid() string {
	// Some abbreviations to save typing.
	shorten := logging.ShortenComponent
	commas := func(xs []string) string { return strings.Join(xs, ", ") }

	// Gather the set of all ops and replicas.
	type replica struct {
		component string
		replica   int
	}
	var ops []OpStart
	replicas := map[replica]struct{}{}
	calls := map[int]Call{}
	returns := map[int]Return{}
	for _, event := range r.History {
		switch x := event.(type) {
		case OpStart:
			ops = append(ops, x)
		case Call:
			calls[x.SpanID] = x
		case DeliverCall:
			call := calls[x.SpanID]
			replicas[replica{call.Component, x.Replica}] = struct{}{}
		case Return:
			returns[x.SpanID] = x
		}
	}

	// Create the diagram.
	var b strings.Builder
	fmt.Fprintln(&b, "sequenceDiagram")

	// Create ops.
	for _, op := range ops {
		fmt.Fprintf(&b, "    participant op%d as Op %d\n", op.TraceID, op.TraceID)
	}

	// Create component replicas.
	sorted := maps.Keys(replicas)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].component != sorted[j].component {
			return sorted[i].component < sorted[j].component
		}
		return sorted[i].replica < sorted[j].replica
	})
	for _, replica := range sorted {
		fmt.Fprintf(&b, "    participant %s%d as %s %d\n", replica.component, replica.replica, shorten(replica.component), replica.replica)
	}

	// Create events.
	for _, event := range r.History {
		switch x := event.(type) {
		case OpStart:
			fmt.Fprintf(&b, "    note right of op%d: [%d:%d] %s(%s)\n", x.TraceID, x.TraceID, x.SpanID, x.Name, commas(x.Args))
		case OpFinish:
			fmt.Fprintf(&b, "    note right of op%d: [%d:%d] return %s\n", x.TraceID, x.TraceID, x.SpanID, x.Error)
		case DeliverCall:
			call := calls[x.SpanID]
			fmt.Fprintf(&b, "    %s%d->>%s%d: [%d:%d] %s.%s(%s)\n", call.Caller, call.Replica, call.Component, x.Replica, x.TraceID, x.SpanID, shorten(call.Component), call.Method, commas(call.Args))
		case DeliverReturn:
			call := calls[x.SpanID]
			ret := returns[x.SpanID]
			fmt.Fprintf(&b, "    %s%d->>%s%d: [%d:%d] return %s\n", ret.Component, ret.Replica, call.Caller, call.Replica, x.TraceID, x.SpanID, commas(ret.Returns))
		case DeliverError:
			call := calls[x.SpanID]
			fmt.Fprintf(&b, "    note right of %s%d: [%d:%d] RemoteCallError\n", call.Caller, call.Replica, x.TraceID, x.SpanID)
		}
	}
	return b.String()
}
