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
	"reflect"
	"runtime/debug"
	"sync"
	"testing"

	core "github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/weaver"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"golang.org/x/sync/errgroup"
)

// TODO(mwhittaker): Here is a list of potential future optimizations. Note
// that the simulator is currently relatively fast, and the following
// optimizations introduce a fair bit of complexity.
//
// - An executor can cache the methods of every component to avoid calling
//   MethodByName for every method call. MethodByName takes a non-trivial
//   amount of time.
// - We can generate code to execute component method calls from a slice of
//   input arguments as []any. This avoids a reflect.Call.
// - We can introduce a new generator interface that returns reflect.Values
//   directly. This allows us to generate values without calling reflect.Call.
// - Currently, an executor records the history of every execution. However,
//   the vast majority of executions pass, and for these passing executions,
//   the history is not needed. We could execute with history disabled, and
//   when we find a failing execution, re-run it with history enabled. If a
//   user's workload isn't deterministic, however, this may lead to failing
//   executions without history.

// componentInfo includes information about components.
type componentInfo struct {
	hasRefs      map[reflect.Type]bool // does a component have weaver.Refs?
	hasListeners map[reflect.Type]bool // does a component have weaver.Listeners?
	hasConfig    map[reflect.Type]bool // does a component have a weaver.Config?
}

// hyperparameters configure an execution.
type hyperparameters struct {
	Seed        int64   // the executor's seed
	NumReplicas int     // the number of replicas of every component
	NumOps      int     // the number of ops to run
	FailureRate float64 // the fraction of calls to artificially fail
	YieldRate   float64 // the probability that an op yields after a step
}

// generator is an untyped Generator[T].
type generator func(*rand.Rand) reflect.Value

// An op is a randomized operation performed as part of an execution. ops are
// derived from a workload's exported methods.
type op struct {
	m          reflect.Method // the op itself
	generators []generator    // the op's generators
}

// An executor deterministically executes a Service Weaver application. An
// executor is not safe for concurrent use by multiple goroutines. However, an
// executor can be used to peform many executions.
//
//	e := newExecutor(...)
//	for {
//	    result, err := e.execute(...)
//	    ...
//	}
type executor struct {
	// Immutable fields.
	w          reflect.Type                           // workload type
	regsByIntf map[reflect.Type]*codegen.Registration // registrations, by component interface
	info       componentInfo                          // component information
	config     *protos.AppConfig                      // application config

	registrar  *registrar       // registrar
	params     hyperparameters  // hyperparameters
	workload   reflect.Value    // workload instance
	ops        []*op            // registered ops
	components map[string][]any // component replicas

	ctx   context.Context // execution context
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

// result is the result of an execution.
type result struct {
	params  hyperparameters // input hyperparameters
	err     error           // first non-nil error returned by an op
	history []Event         // a history of the execution, if err is not nil
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
	failAfterDelivery

	// TODO(mwhittaker): Deliver a method call, and then fail it while it is
	// still executing.
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

// newExecutor returns a new executor.
func newExecutor(t testing.TB, w reflect.Type, regsByIntf map[reflect.Type]*codegen.Registration, info componentInfo, app *protos.AppConfig) *executor {
	registered := map[reflect.Type]struct{}{}
	for intf := range regsByIntf {
		registered[intf] = struct{}{}
	}
	return &executor{
		w:          w,
		regsByIntf: regsByIntf,
		info:       info,
		config:     app,
		registrar:  newRegistrar(t, w, registered),
		components: make(map[string][]any, len(regsByIntf)),
		rand:       rand.New(&wyrand{0}),
		calls:      map[int][]*call{},
		replies:    map[int][]*reply{},
	}
}

// execute performs a single execution. execute returns an error if the
// execution fails to run properly. If the execution runs properly, any error
// produced by the workload is returned in the result struct.
//
//	result, err := e.execute(...)
//	switch {
//	    case err != nil:
//	        // The execution did not run properly.
//	    case result.err != nil:
//	        // The execution ran properly; the workload returned an error.
//	    default:
//	        // The execution ran properly; the workload did not return an error.
//	}
func (e *executor) execute(ctx context.Context, params hyperparameters) (result, error) {
	// Validate hyperparameters.
	if params.NumReplicas <= 0 {
		return result{}, fmt.Errorf("NumReplicas (%d) <= 0", params.NumReplicas)
	}
	if params.NumOps <= 0 {
		return result{}, fmt.Errorf("NumOps (%d) <= 0", params.NumOps)
	}
	if params.FailureRate < 0 || params.FailureRate > 1 {
		return result{}, fmt.Errorf("FailureRate (%f) out of range [0, 1]", params.FailureRate)
	}
	if params.YieldRate < 0 || params.YieldRate > 1 {
		return result{}, fmt.Errorf("YieldRate (%f) out of range [0, 1]", params.YieldRate)
	}

	// Construct an instance of the workload struct.
	workload := reflect.New(e.w.Elem()).Interface().(Workload)

	// Register fakes and components.
	e.registrar.reset()
	if err := workload.Init(e.registrar); err != nil {
		return result{}, err
	}
	if err := e.registrar.finalize(); err != nil {
		return result{}, err
	}

	// Reset the executor.
	fakes := e.registrar.fakes
	ops := e.registrar.ops
	if err := e.reset(workload, fakes, ops, params); err != nil {
		return result{}, err
	}

	// Perform the execution.
	e.group, e.ctx = errgroup.WithContext(ctx)
	e.step()
	err := e.group.Wait()
	if err != nil && err == ctx.Err() {
		return result{}, err
	}
	return result{params, err, e.history}, nil
}

// reset resets the state of an executor, preparing it for the next execution.
func (e *executor) reset(workload Workload, fakes map[reflect.Type]any, ops []*op, params hyperparameters) error {
	e.workload = reflect.ValueOf(workload)
	e.params = params
	e.ops = ops
	e.rand.Seed(params.Seed)
	e.current = 1
	e.numStarted = 0
	e.notFinished.reset(1, 1+params.NumOps)
	for k, v := range e.calls {
		e.calls[k] = v[:0]
	}
	for k, v := range e.replies {
		e.replies[k] = v[:0]
	}
	e.history = []Event{}
	e.nextTraceID = 1
	e.nextSpanID = 1

	// Fill ref fields inside the workload struct.
	if err := weaver.FillRefs(workload, func(t reflect.Type) (any, error) {
		return e.getIntf(t, "op", 0)
	}); err != nil {
		return err
	}

	// Create component replicas.
	for _, reg := range e.regsByIntf {
		components := e.components[reg.Name]
		if components != nil {
			components = components[:0]
		}

		if fake, ok := fakes[reg.Iface]; ok {
			e.components[reg.Name] = append(components, fake)
			continue
		}

		for i := 0; i < params.NumReplicas; i++ {
			// Create the component implementation.
			v := reflect.New(reg.Impl)
			obj := v.Interface()

			// Fill config.
			if e.info.hasConfig[reg.Iface] {
				if cfg := weaver.GetConfig(obj); cfg != nil {
					if err := runtime.ParseConfigSection(reg.Name, "", e.config.Sections, cfg); err != nil {
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
			if e.info.hasRefs[reg.Iface] {
				if err := weaver.FillRefs(obj, func(t reflect.Type) (any, error) {
					return e.getIntf(t, reg.Name, i)
				}); err != nil {
					return err
				}
			}

			// Fill listener fields.
			if e.info.hasListeners[reg.Iface] {
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
		e.components[reg.Name] = components
	}

	return nil
}

// getIntf returns a handle to the component of the provided type.
func (e *executor) getIntf(t reflect.Type, caller string, replica int) (any, error) {
	reg, ok := e.regsByIntf[t]
	if !ok {
		return nil, fmt.Errorf("component %v not found", t)
	}
	call := func(method string, ctx context.Context, args []any, returns []any) error {
		return e.call(caller, replica, reg, method, ctx, args, returns)
	}
	return reg.ReflectStubFn(call), nil
}

// call executes a component method call against a random replica.
func (e *executor) call(caller string, replica int, reg *codegen.Registration, method string, ctx context.Context, args []any, returns []any) error {
	// Convert the arguments to reflect.Values.
	in := make([]reflect.Value, 1+len(args))
	in[0] = reflect.ValueOf(ctx)
	strings := make([]string, len(args))
	for i, arg := range args {
		in[i+1] = reflect.ValueOf(arg)
		strings[i] = fmt.Sprint(arg)
	}

	// Extract the trace id.
	traceID, _ := extractIDs(ctx)
	if traceID == 0 {
		// TODO(mwhittaker): Link to online documentation with better
		// explanation of this error.
		panic(fmt.Errorf("missing simulation trace context. Make sure that every component method call is performed with the context the caller was invoked with."))
	}

	// Record the call.
	reply := make(chan *reply, 1)
	e.mu.Lock()
	spanID := e.nextSpanID
	e.nextSpanID++

	// Determine the fate of the call.
	fate := dontFail
	if flip(e.rand, e.params.FailureRate) {
		// TODO(mwhittaker): Have two parameters to control the rate of failing
		// before and after delivery? This level of control might be
		// unnecessary. For now, we pick between them equiprobably.
		if flip(e.rand, 0.5) {
			fate = failBeforeDelivery
		} else {
			fate = failAfterDelivery
		}
	}

	e.calls[traceID] = append(e.calls[traceID], &call{
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
	e.history = append(e.history, EventCall{
		TraceID:   traceID,
		SpanID:    spanID,
		Caller:    caller,
		Replica:   replica,
		Component: reg.Name,
		Method:    method,
		Args:      strings,
	})
	e.mu.Unlock()

	// Take a step and wait for the call to finish.
	e.step()
	var out []reflect.Value
	select {
	case r := <-reply:
		out = r.returns
	case <-e.ctx.Done():
		return e.ctx.Err()
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

// step performs one step of an execution.
func (e *executor) step() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.ctx.Err() != nil {
		// The execution has been cancelled.
		return
	}

	if e.notFinished.size() == 0 {
		// The execution is finished.
		return
	}

	if !e.notFinished.has(e.current) || flip(e.rand, e.params.YieldRate) {
		// Yield execution to a (potentially) different op.
		e.current = e.notFinished.pick(e.rand)
	}

	if e.current > e.numStarted {
		// Make sure to start ops in increasing order. Op 1 starts first, then
		// Op 2, and so on.
		e.current = e.numStarted + 1
		e.numStarted++

		// Start the op.
		o := pick(e.rand, e.ops)
		e.group.Go(func() error {
			return e.runOp(e.ctx, o)
		})
		return
	}

	if len(e.calls[e.current]) == 0 && len(e.replies[e.current]) == 0 {
		// This should be impossible. If it ever happens, there's a bug.
		panic(fmt.Errorf("op %d has no pending calls or replies", e.current))
	}

	hasCalls := len(e.calls[e.current]) > 0
	hasReplies := len(e.replies[e.current]) > 0
	deliverCall := false
	switch {
	case hasCalls && hasReplies:
		deliverCall = flip(e.rand, 0.5)
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
		call, e.calls[e.current] = pop(e.rand, e.calls[e.current])

		if call.fate == failBeforeDelivery {
			// Fail the call before delivering it.
			e.history = append(e.history, EventDeliverError{
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
		e.group.Go(func() error {
			return e.deliverCall(call)
		})
	} else {
		var reply *reply
		reply, e.replies[e.current] = pop(e.rand, e.replies[e.current])

		if reply.call.fate == failAfterDelivery {
			// Fail the call after delivering it.
			e.history = append(e.history, EventDeliverError{
				TraceID: reply.call.traceID,
				SpanID:  reply.call.spanID,
			})
			reply.returns = returnError(reply.call.component, reply.call.method, core.RemoteCallError)
			reply.call.reply <- reply
			close(reply.call.reply)
			return
		}

		// Return successfully.
		e.history = append(e.history, EventDeliverReturn{
			TraceID: reply.call.traceID,
			SpanID:  reply.call.spanID,
		})
		reply.call.reply <- reply
		close(reply.call.reply)
	}
}

// runOp runs the provided operation.
func (e *executor) runOp(ctx context.Context, o *op) (err error) {
	var traceID, spanID int
	defer func() {
		if x := recover(); x != nil {
			err = fmt.Errorf("panic: %v", x)
			e.mu.Lock()
			e.history = append(e.history, EventPanic{
				TraceID:  traceID,
				SpanID:   spanID,
				Panicker: "op",
				Replica:  traceID,
				Error:    err.Error(),
				Stack:    string(debug.Stack()),
			})
			e.mu.Unlock()
		}
	}()

	// Allocate args on the stack when possible.
	var args []reflect.Value
	n := 2 + len(o.generators)
	if n <= 20 {
		var buf [20]reflect.Value
		args = buf[:n]
	} else {
		args = make([]reflect.Value, n)
	}
	formatted := make([]string, len(o.generators))

	e.mu.Lock()
	traceID, spanID = e.nextTraceID, e.nextSpanID
	e.nextTraceID++
	e.nextSpanID++

	// Generate random op inputs. Lock s.mu because s.rand is not safe for
	// concurrent use by multiple goroutines.
	args[0] = e.workload
	args[1] = reflect.ValueOf(withIDs(ctx, traceID, spanID))
	for i, generator := range o.generators {
		x := generator(e.rand)
		args[i+2] = x
		formatted[i] = fmt.Sprint(x.Interface())
	}

	// Record an OpStart event.
	e.history = append(e.history, EventOpStart{
		TraceID: traceID,
		SpanID:  spanID,
		Name:    o.m.Name,
		Args:    formatted,
	})
	e.mu.Unlock()

	// Invoke the op.
	if x := o.m.Func.Call(args)[0].Interface(); x != nil {
		err = x.(error)
	}

	if e.ctx.Err() != nil {
		// The simulation was cancelled. Abort.
		return e.ctx.Err()
	}

	// Record an OpFinish event.
	msg := "<nil>"
	if err != nil {
		msg = err.Error()
	}
	e.mu.Lock()
	e.history = append(e.history, EventOpFinish{
		TraceID: traceID,
		SpanID:  spanID,
		Error:   msg,
	})
	e.notFinished.remove(traceID)
	e.mu.Unlock()

	if err != nil {
		// If an op returns a non-nil error, abort the execution and don't
		// take another step. Because the runOp function is running inside an
		// errgroup.Group, the returned error will cancel all goroutines.
		return err
	}

	// If the op succeeded, take another step.
	e.step()
	return nil
}

// deliverCall delivers the provided pending method call.
func (e *executor) deliverCall(call *call) (err error) {
	var component string
	var index int
	defer func() {
		if x := recover(); x != nil {
			err = fmt.Errorf("panic: %v", x)
			e.mu.Lock()
			e.history = append(e.history, EventPanic{
				TraceID:  call.traceID,
				SpanID:   call.spanID,
				Panicker: component,
				Replica:  index,
				Error:    err.Error(),
				Stack:    string(debug.Stack()),
			})
			e.mu.Unlock()
		}
	}()

	reg, ok := e.regsByIntf[call.component]
	if !ok {
		return fmt.Errorf("component %v not found", call.component)
	}

	// Pick a replica to execute the call.
	component = reg.Name
	replicas := e.components[component]
	e.mu.Lock()
	index = e.rand.Intn(len(replicas))
	replica := replicas[index]

	// Record a DeliverCall event.
	e.history = append(e.history, EventDeliverCall{
		TraceID:   call.traceID,
		SpanID:    call.spanID,
		Component: reg.Name,
		Replica:   index,
	})
	e.mu.Unlock()

	// Call the component method.
	returns := reflect.ValueOf(replica).MethodByName(call.method).Call(call.args)
	strings := make([]string, len(returns))
	for i, ret := range returns {
		strings[i] = fmt.Sprint(ret.Interface())
	}

	if e.ctx.Err() != nil {
		// The simulation was cancelled. Abort.
		return e.ctx.Err()
	}

	// Record the reply and take a step.
	e.mu.Lock()
	e.replies[call.traceID] = append(e.replies[call.traceID], &reply{
		call:    call,
		returns: returns,
	})

	e.history = append(e.history, EventReturn{
		TraceID:   call.traceID,
		SpanID:    call.spanID,
		Component: reg.Name,
		Replica:   index,
		Returns:   strings,
	})
	e.mu.Unlock()
	e.step()
	return nil
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
