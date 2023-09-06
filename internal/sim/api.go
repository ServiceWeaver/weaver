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

// Package sim...
//
// TODO(mwhittaker): Write comprehensive package documentation with examples.
// We also probably want to put some of this documentation on the website, and
// we might also want to write a blog.
//
// TODO(mwhittaker): Move things to the weavertest package.
package sim

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/weavertest"
)

// A Generator[T] generates random values of type T.
type Generator[T any] interface {
	// Generate returns a randomly generated value of type T. While Generate is
	// "random", it must be deterministic. That is, given the same instance of
	// *rand.Rand, Generate must always return the same value.
	Generate(*rand.Rand) T
}

// A Registrar is used to register stuff with a simulator.
type Registrar interface {
	// RegisterFake registers a fake implementation of a component.
	RegisterFake(weavertest.FakeComponent)

	// RegisterGenerators registers generators for a workload method, one
	// generator per method argument. The number and type of the registered
	// generators must match the method. For example, given the method:
	//
	//     Foo(context.Context, int, bool) error
	//
	// we must register a Generator[int] and a Generator[bool]:
	//
	//     var r Registrar = ...
	//     var i Generator[int] = ...
	//     var b Generator[bool] = ...
	//     r.RegisterGenerators("Foo", i, b)
	RegisterGenerators(method string, generators ...any)
}

// A Workload defines the set of operations to run as part of a simulation.
// Every workload is defined as a named struct. To perform a simulation, a
// simulator constructs an instance of the struct, calls the struct's Init
// method, and then randomly calls the struct's exported methods. For example,
// the following is a simple workload:
//
//	type workload struct {}
//	func (w *workload) Init(r sim.Registrar) {...}
//	func (w *workload) Foo(context.Context, int) error {...}
//	func (w *workload) Bar(context.Context, bool, string) error {...}
//	func (w *workload) baz(context.Context) error {...}
//
// When this workload is simulated, its Foo and Bar methods will be called with
// random values generated by the generators registered in the Init method (see
// [Registrar] for details). Note that unexported methods, like baz, are
// ignored.
//
// Note that every exported workload method must receive a context.Context as
// its first argument and must return a single error value. A simulation is
// aborted when a method returns a non-nil error.
//
// TODO(mwhittaker): For now, the Init method is required. In the future, we
// could make it optional and use default generators for methods.
type Workload interface {
	Init(Registrar) error
}

// WorkloadPointer[T] is a *T that implements the Workload interface.
type WorkloadPointer[T any] interface {
	*T
	Workload
}

// Options configure a Simulator.
type Options struct {
	Config string // TOML config contents
}

// A Simulator deterministically simulates a Service Weaver application. See
// the package documentation for instructions on how to use a Simulator.
type Simulator struct{}

// An Event represents an atomic step of a simulation.
type Event interface {
	isEvent()
}

// TODO(mwhittaker): Prefix all events with Event so they show up together in
// godoc? It might also make it clearer that they are events.

// OpStart represents the start of an op.
type OpStart struct {
	TraceID int      // trace id
	SpanID  int      // span id
	Name    string   // op name
	Args    []string // op arguments
}

// OpFinish represents the finish of an op.
type OpFinish struct {
	TraceID int    // trace id
	SpanID  int    // span id
	Error   string // returned error message
}

// Call represents a component method call.
type Call struct {
	TraceID   int      // trace id
	SpanID    int      // span id
	Caller    string   // calling component (or "op")
	Replica   int      // calling component replica (or op number)
	Component string   // component being called
	Method    string   // method being called
	Args      []string // method arguments
}

// DeliverCall represents a component method call being delivered.
type DeliverCall struct {
	TraceID   int    // trace id
	SpanID    int    // span id
	Component string // component being called
	Replica   int    // component replica being called
}

// Return represents a component method call returning.
type Return struct {
	TraceID   int      // trace id
	SpanID    int      // span id
	Component string   // component returning
	Replica   int      // component replica returning
	Returns   []string // return values
}

// DeliverReturn represents the delivery of a method return.
type DeliverReturn struct {
	TraceID int // trace id
	SpanID  int // span id
}

// DeliverError represents the injection of an error.
type DeliverError struct {
	TraceID int // trace id
	SpanID  int // span id
}

func (OpStart) isEvent()       {}
func (OpFinish) isEvent()      {}
func (Call) isEvent()          {}
func (DeliverCall) isEvent()   {}
func (Return) isEvent()        {}
func (DeliverReturn) isEvent() {}
func (DeliverError) isEvent()  {}

var _ Event = OpStart{}
var _ Event = OpFinish{}
var _ Event = Call{}
var _ Event = DeliverCall{}
var _ Event = Return{}
var _ Event = DeliverReturn{}
var _ Event = DeliverError{}

// Results are the results of running a simulation.
type Results struct {
	Err     error   // first non-nil error returned by an op
	History []Event // a history of all simulation events
}

// New returns a new Simulator that simulates workload W.
func New[W any, P WorkloadPointer[W]](t *testing.T) *Simulator {
	// Note that because the Init method on a workload struct T often has a
	// pointer receiver *T, it is the pointer *T (rather than T itself) that
	// implements the Workload interface.
	t.Helper()

	// Validate the workload struct.
	w := reflection.Type[P]()
	if err := validateWorkload(w); err != nil {
		t.Fatalf("sim.New: invalid workload type %v: %v", w, err)
	}

	// Call Init. Validate the registered fakes and generators.
	var x W
	r := &registrar{t: t, w: w}
	if err := P(&x).Init(r); err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	if err := r.finalize(); err != nil {
		t.Fatalf("sim.New: %v", err)
	}
	return &Simulator{}
}

// validateWorkload validates a workload struct of the provided type.
func validateWorkload(t reflect.Type) error {
	var errs []error
	numOps := 0
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if m.Name == "Init" {
			continue
		}
		numOps++

		// Method should have type func(context.Context, ...) error.
		err := fmt.Errorf("method %s has type '%v' but should have type 'func(%v, context.Context, ...) error'", m.Name, m.Type, t)
		switch {
		case m.Type.NumIn() < 2:
			errs = append(errs, fmt.Errorf("%w: no arguments", err))
		case m.Type.In(1) != reflection.Type[context.Context]():
			errs = append(errs, fmt.Errorf("%w: first argument is not context.Context", err))
		case m.Type.NumOut() == 0:
			errs = append(errs, fmt.Errorf("%w: no return value", err))
		case m.Type.NumOut() > 1:
			errs = append(errs, fmt.Errorf("%w: too many return values", err))
		case m.Type.Out(0) != reflection.Type[error]():
			errs = append(errs, fmt.Errorf("%w: return value is not error", err))
		}
	}
	if numOps == 0 {
		errs = append(errs, fmt.Errorf("no exported methods"))
	}
	return errors.Join(errs...)
}

// Run runs simulations for the provided duration.
func (s *Simulator) Run(duration time.Duration, opts Options) Results {
	// TODO(mwhittaker): Implement.
	return Results{}
}

// A registrar is the canonical implementation of Registrar.
//
// TODO(mwhittaker): For now, the registrar just validates registered fakes and
// generators. In the future, it will do more than that.
type registrar struct {
	t          *testing.T          // underlying test
	w          reflect.Type        // workload type
	generators map[string]struct{} // methods with registered generators
}

// RegisterFake implements the Registrar interface.
func (r *registrar) RegisterFake(weavertest.FakeComponent) {
	// TODO(mwhittaker): Implement.
}

// RegisterGenerators implements the Registrar interface.
func (r *registrar) RegisterGenerators(method string, generators ...any) {
	r.t.Helper()
	if err := r.registerGenerators(method, generators...); err != nil {
		r.t.Fatalf("RegisterGenerators: %v", err)
	}
}

// registerGenerators implements RegisterGenerators.
func (r *registrar) registerGenerators(method string, generators ...any) error {
	if _, ok := r.generators[method]; ok {
		return fmt.Errorf("method %q generators already registered", method)
	}
	m, ok := r.w.MethodByName(method)
	if !ok {
		return fmt.Errorf("method %q not found", method)
	}
	if got, want := len(generators), m.Type.NumIn()-2; got != want {
		return fmt.Errorf("method %v: want %d generators, got %d", method, want, got)
	}

	var errs []error
	for i, generator := range generators {
		// TODO(mwhittaker): Handle the case where a generator's Generate
		// method receives by pointer, but the user passed by value.
		t := reflect.TypeOf(generator)
		err := fmt.Errorf("method %s generator %d is not a generator", method, i)
		if t == nil {
			errs = append(errs, fmt.Errorf("%w: missing Generate method", err))
			continue
		}

		generate, ok := t.MethodByName("Generate")
		switch {
		case !ok:
			errs = append(errs, fmt.Errorf("%w: missing Generate method", err))
		case generate.Type.NumIn() < 2:
			errs = append(errs, fmt.Errorf("%w: Generate method has no arguments", err))
		case generate.Type.NumIn() > 2:
			errs = append(errs, fmt.Errorf("%w: Generate method has too many arguments", err))
		case generate.Type.In(1) != reflection.Type[*rand.Rand]():
			errs = append(errs, fmt.Errorf("%w: Generate argument is not *rand.Rand", err))
		case generate.Type.NumOut() == 0:
			errs = append(errs, fmt.Errorf("%w: Generate method has no return values", err))
		case generate.Type.NumOut() > 1:
			errs = append(errs, fmt.Errorf("%w: Generate method has too many return values", err))
		case generate.Type.Out(0) != m.Type.In(i+2):
			errs = append(errs, fmt.Errorf("method %s invalid generator %d: got Generator[%v], want Generator[%v]", method, i, generate.Type.Out(0), m.Type.In(i+2)))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if r.generators == nil {
		r.generators = map[string]struct{}{}
	}
	r.generators[method] = struct{}{}
	return nil
}

// finalize finalizes registration.
func (r *registrar) finalize() error {
	var errs []error
	for i := 0; i < r.w.NumMethod(); i++ {
		m := r.w.Method(i)
		if _, ok := r.generators[m.Name]; m.Name != "Init" && !ok {
			errs = append(errs, fmt.Errorf("no generators registered for method %s", m.Name))
		}
	}
	return errors.Join(errs...)
}