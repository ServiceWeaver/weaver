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
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/reflection"
)

// registrar is the canonical Registrar implementation. A registrar is not safe
// for concurrent use by multiple goroutines. However, a registrar can be used
// across multiple executions by calling the reset method.
//
//	r := newRegistrar(...)
//	for {
//	    r.RegisterFakes(...)
//	    r.RegisterGenerators(...)
//	    r.finalize()
//	    r.reset()
//	}
type registrar struct {
	// Immutable fields.
	t          testing.TB                // underlying test
	registered map[reflect.Type]struct{} // registered component interfaces
	opsByName  map[string]int            // index into ops, by op name

	// Cached after the first execution.
	typeInfo map[string][]generatorTypeInfo // generator type info

	// Updated for every execution.
	fakes map[reflect.Type]any // fakes, by component interface
	ops   []*op                // operations
}

var _ Registrar = &registrar{}

// generatorTypeInfo holds type information about a generator.
type generatorTypeInfo struct {
	t        reflect.Type   // the type of the generator
	generate reflect.Method // the Generate method
}

// newRegistrar returns a new registrar.
func newRegistrar(t testing.TB, w reflect.Type, registered map[reflect.Type]struct{}) *registrar {
	// Gather the set of ops.
	ops := []*op{}
	opsByName := map[string]int{}
	for i := 0; i < w.NumMethod(); i++ {
		m := w.Method(i)
		if m.Name == "Init" {
			continue
		}
		arity := m.Type.NumIn() - 2 // ignore receiver and context arguments
		ops = append(ops, &op{m, make([]generator, 0, arity)})
		opsByName[m.Name] = len(ops) - 1
	}

	return &registrar{
		t:          t,
		registered: registered,
		fakes:      map[reflect.Type]any{},
		typeInfo:   map[string][]generatorTypeInfo{},
		ops:        ops,
		opsByName:  opsByName,
	}
}

// reset resets a registrar.
func (r *registrar) reset() {
	for k := range r.fakes {
		delete(r.fakes, k)
	}
	for _, op := range r.ops {
		op.generators = op.generators[:0]
	}
}

// RegisterFake implements the Registrar interface.
func (r *registrar) RegisterFake(fake FakeComponent) {
	r.t.Helper()
	if err := r.registerFakes(fake); err != nil {
		r.t.Fatalf("RegisterFakes: %v", err)
	}
}

// RegisterGenerators implements the Registrar interface.
func (r *registrar) RegisterGenerators(method string, generators ...any) {
	r.t.Helper()
	if err := r.registerGenerators(method, generators...); err != nil {
		r.t.Fatalf("RegisterGenerators: %v", err)
	}
}

// registerFakes implements RegisterFakes.
func (r *registrar) registerFakes(fake FakeComponent) error {
	if _, ok := r.fakes[fake.intf]; ok {
		return fmt.Errorf("fake for %v already registered", fake.intf)
	}
	if _, ok := r.registered[fake.intf]; !ok {
		return fmt.Errorf("component %v not found", fake.intf)
	}
	r.fakes[fake.intf] = fake.impl
	return nil
}

// registerGenerators implements RegisterGenerators.
func (r *registrar) registerGenerators(method string, generators ...any) error {
	i, ok := r.opsByName[method]
	if !ok {
		return fmt.Errorf("method %q not found", method)
	}
	op := r.ops[i]
	if len(op.generators) > 0 {
		return fmt.Errorf("method %q generators already registered", method)
	}
	arity := op.m.Type.NumIn() - 2 // ignore receiver and context arguments
	if len(generators) != arity {
		return fmt.Errorf("method %v: want %d generators, got %d", method, arity, len(generators))
	}

	if infos, ok := r.typeInfo[method]; ok {
		// We have type information cached for this method.
		var errs []error
		for i, generator := range generators {
			info := infos[i]
			t := reflect.TypeOf(generator)
			if t != info.t {
				errs = append(errs, fmt.Errorf("method %s generator %d has type %v, but previously had type %v", method, i, t, info.t))
				continue
			}
			generator := generator
			op.generators = append(op.generators, func(r *rand.Rand) reflect.Value {
				in := []reflect.Value{reflect.ValueOf(generator), reflect.ValueOf(r)}
				return info.generate.Func.Call(in)[0]
			})
		}
		return errors.Join(errs...)
	}

	// We don't have type information cached for this method.
	var errs []error
	infos := make([]generatorTypeInfo, arity)
	for i, generator := range generators {
		// TODO(mwhittaker): Handle the case where a generator's Generate
		// method receives by pointer, but the user passed by value.
		t := reflect.TypeOf(generator)
		err := func() error {
			return fmt.Errorf("method %s generator %d is not a generator", method, i)
		}
		if t == nil {
			errs = append(errs, fmt.Errorf("%w: missing Generate method", err()))
			continue
		}

		// TODO(mwhittaker): If the Generator interface looked like this:
		//
		//     type Generator[T any] interface {
		//         Generate(*rand.Rand) any
		//     }
		//
		// then we could write `g, ok := generator.(Generator)`, which is much
		// faster. Think about how to speed things up without making the
		// interface ugly.
		generate, ok := t.MethodByName("Generate")
		switch {
		case !ok:
			errs = append(errs, fmt.Errorf("%w: missing Generate method", err()))
			continue
		case generate.Type.NumIn() < 2:
			errs = append(errs, fmt.Errorf("%w: Generate method has no arguments", err()))
			continue
		case generate.Type.NumIn() > 2:
			errs = append(errs, fmt.Errorf("%w: Generate method has too many arguments", err()))
			continue
		case generate.Type.In(1) != reflection.Type[*rand.Rand]():
			errs = append(errs, fmt.Errorf("%w: Generate argument is not *rand.Rand", err()))
			continue
		case generate.Type.NumOut() == 0:
			errs = append(errs, fmt.Errorf("%w: Generate method has no return values", err()))
			continue
		case generate.Type.NumOut() > 1:
			errs = append(errs, fmt.Errorf("%w: Generate method has too many return values", err()))
			continue
		case generate.Type.Out(0) != op.m.Type.In(i+2):
			errs = append(errs, fmt.Errorf("method %s invalid generator %d: got Generator[%v], want Generator[%v]", method, i, generate.Type.Out(0), op.m.Type.In(i+2)))
			continue
		}

		generator := generator
		infos[i] = generatorTypeInfo{t, generate}
		op.generators = append(op.generators, func(r *rand.Rand) reflect.Value {
			in := []reflect.Value{reflect.ValueOf(generator), reflect.ValueOf(r)}
			return generate.Func.Call(in)[0]
		})
	}
	err := errors.Join(errs...)
	if err == nil {
		r.typeInfo[method] = infos
	}
	return err
}

// finalize finalizes registration.
func (r *registrar) finalize() error {
	var errs []error
	for _, op := range r.ops {
		arity := op.m.Type.NumIn() - 2 // ignore receiver and context arguments
		if len(op.generators) != arity {
			errs = append(errs, fmt.Errorf("no generators registered for method %s", op.m.Name))
		}
	}
	return errors.Join(errs...)
}
