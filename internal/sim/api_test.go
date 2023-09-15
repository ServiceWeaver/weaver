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
	"errors"
	"io"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

// TODO(mwhittaker): Move these into the sim package.
type integers struct{}
type positives struct{}
type intn struct{ low, high int }
type float64s struct{}

var _ Generator[int] = integers{}
var _ Generator[int] = positives{}
var _ Generator[int] = intn{}
var _ Generator[float64] = float64s{}

func (integers) Generate(r *rand.Rand) int     { return r.Int() }
func (positives) Generate(r *rand.Rand) int    { return 1 + r.Intn(math.MaxInt) }
func (i intn) Generate(r *rand.Rand) int       { return i.low + r.Intn(i.high-i.low) }
func (float64s) Generate(r *rand.Rand) float64 { return r.Float64() }

type divModWorkload struct {
	divmod weaver.Ref[divMod]
	div    weaver.Ref[div]
	mod    weaver.Ref[mod]
}

func (d *divModWorkload) Init(r Registrar) error {
	r.RegisterGenerators("DivMod", integers{}, positives{})
	r.RegisterGenerators("Div", integers{}, positives{})
	r.RegisterGenerators("Mod", integers{}, positives{})
	return nil
}

func (d *divModWorkload) DivMod(ctx context.Context, x, y int) error {
	d.divmod.Get().DivMod(ctx, x, y)
	return nil
}

func (d *divModWorkload) Div(ctx context.Context, x, y int) error {
	d.div.Get().Div(ctx, x, y)
	return nil
}

func (d *divModWorkload) Mod(ctx context.Context, x, y int) error {
	d.mod.Get().Mod(ctx, x, y)
	return nil
}

func TestPassingSimulation(t *testing.T) {
	for _, test := range []struct {
		name     string
		workload Workload
	}{
		{"NoCallsNoGen", &noCallsNoGenWorkload{}},
		{"NoCalls", &noCallsWorkload{}},
		{"OneCall", &oneCallWorkload{}},
		{"DivMod", &divModWorkload{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := New(t, test.workload, Options{})
			r := s.Run(2 * time.Second)
			t.Log(r.Summary())
			if r.Err != nil {
				t.Fatal(r.Err)
			}
		})
	}
}

// See TestInitByValueSimulation.
type initByValueWorkload struct{}

func (initByValueWorkload) Init(r Registrar) error {
	r.RegisterGenerators("ByValue")
	r.RegisterGenerators("ByPointer")
	return nil
}

func (initByValueWorkload) ByValue(context.Context) error {
	return nil
}

func (*initByValueWorkload) ByPointer(context.Context) error {
	return nil
}

func TestInitByValueSimulation(t *testing.T) {
	// Run a simulation where the workload's Init method has a value receiver.
	// Make sure that all methods of the workload are called.
	s := New(t, initByValueWorkload{}, Options{})
	r, err := s.simulateOne(context.Background(), s.newRegistrar(), s.newSimulator(), options{
		NumReplicas: 1,
		NumOps:      1000,
		FailureRate: 0,
		YieldRate:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Err != nil {
		t.Fatal(r.Err)
	}

	byValueCalls := 0
	byPointerCalls := 0
	for _, event := range r.History {
		op, ok := event.(EventOpStart)
		if !ok {
			continue
		}
		switch op.Name {
		case "ByValue":
			byValueCalls++
		case "ByPointer":
			byPointerCalls++
		}
	}
	if byValueCalls == 0 {
		t.Fatal("ByValue not called")
	}
	if byPointerCalls == 0 {
		t.Fatal("ByPointer not called")
	}
}

type divideByZeroWorkload struct {
	divmod weaver.Ref[divMod]
}

func (d *divideByZeroWorkload) Init(r Registrar) error {
	r.RegisterGenerators("DivMod", intn{0, 10}, intn{0, 10})
	return nil
}

func (d *divideByZeroWorkload) DivMod(ctx context.Context, x, y int) error {
	_, _, err := d.divmod.Get().DivMod(ctx, x, y)
	if errors.Is(err, weaver.RemoteCallError) {
		// Swallow remote call errors.
		return nil
	}
	return err
}

func TestFailingSimulation(t *testing.T) {
	s := New(t, &divideByZeroWorkload{}, Options{})
	r := s.Run(2 * time.Second)
	t.Log(r.Summary())
	if r.Err == nil {
		t.Fatal("Unexpected success")
	}
}

func TestValidateValidWorkload(t *testing.T) {
	// Call validateWorkload on a valid workload.
	w := reflect.PointerTo(reflection.Type[divModWorkload]())
	if err := validateWorkload(w); err != nil {
		t.Fatal(err)
	}
}

// invalidWorkload is an invalid workload. See TestValidateInvalidWorkload.
type invalidWorkload struct{}

func (*invalidWorkload) NoArguments() error                           { return nil }
func (*invalidWorkload) WrongFirstArgument(int) error                 { return nil }
func (*invalidWorkload) NoReturns(context.Context)                    {}
func (*invalidWorkload) TooManyReturnss(context.Context) (int, error) { return 0, nil }
func (*invalidWorkload) WrongReturn(context.Context) int              { return 0 }

func TestValidateInvalidWorkload(t *testing.T) {
	// Call validateWorkload on an invalid workload.
	w := reflect.PointerTo(reflection.Type[invalidWorkload]())
	err := validateWorkload(w)
	if err == nil {
		t.Fatal("unexpected success")
	}
	for _, want := range []string{
		"no arguments",
		"first argument is not context.Context",
		"no return value",
		"too many return values",
		"return value is not error",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Error does not contain %q:\n%s", want, err.Error())
		}
	}
}

func TestValidateEmptyWorkload(t *testing.T) {
	// Call validateWorkload on a workload with no exported methods.
	type emptyWorkload struct{}
	w := reflect.PointerTo(reflection.Type[emptyWorkload]())
	err := validateWorkload(w)
	if err == nil {
		t.Fatal("unexpected success")
	}
	const want = "no exported methods"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("Error does not contain %q:\n%s", want, err.Error())
	}
}

func TestValidateIllTypedWorkloads(t *testing.T) {
	// Call validateWorkload on types that aren't named structs.
	for _, w := range []reflect.Type{
		reflection.Type[int](),
		reflection.Type[struct{}](),
		reflection.Type[io.Reader](),
	} {
		t.Run(w.String(), func(t *testing.T) {
			err := validateWorkload(w)
			if err == nil {
				t.Errorf("unexpected success")
			}
		})
	}
}

func TestDuplicateRegisterGeneratorsCalls(t *testing.T) {
	// Call registerGenerators twice for the same method.
	w := reflect.PointerTo(reflection.Type[divModWorkload]())
	r := newTestRegistrar(t, w)
	if err := r.registerGenerators("DivMod", integers{}, positives{}); err != nil {
		t.Fatal(err)
	}
	err := r.registerGenerators("DivMod", integers{}, positives{})
	if err == nil {
		t.Fatal("unexpected success")
	}
	const want = "already registered"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("Error does not contain %q:\n%s", want, err.Error())
	}
}

// Invalid generators. See TestRegisterInvalidGenerators.
type noArguments struct{}
type tooManyArguments struct{}
type nonRandArgument struct{}
type noReturn struct{}
type tooManyReturns struct{}

func (noArguments) Generate() error                     { return nil }
func (tooManyArguments) Generate(int, *rand.Rand) error { return nil }
func (nonRandArgument) Generate(int) error              { return nil }
func (noReturn) Generate(*rand.Rand)                    {}
func (tooManyReturns) Generate(*rand.Rand) (int, error) { return 0, nil }

func TestRegisterInvalidGenerators(t *testing.T) {
	// Call registerGenerators on invalid generators.
	i, p := integers{}, positives{}
	for _, test := range []struct {
		name       string
		method     string
		generators []any
		want       string
	}{
		{"MissingMethod", "Foo", []any{i, p}, "not found"},
		{"TooFewGenerators", "Div", []any{i}, "want 2 generators, got 1"},
		{"TooManyGenerators", "Div", []any{i, i, p}, "want 2 generators, got 3"},
		{"NilGenerator", "Div", []any{i, nil}, "missing Generate method"},
		{"MissingGenerateMethod", "Div", []any{i, struct{}{}}, "missing Generate method"},
		{"NoArguments", "Div", []any{i, noArguments{}}, "no arguments"},
		{"TooManyArguments", "Div", []any{i, tooManyArguments{}}, "too many arguments"},
		{"NonRandArgument", "Div", []any{i, nonRandArgument{}}, "not *rand.Rand"},
		{"NoReturn", "Div", []any{i, noReturn{}}, "no return values"},
		{"TooManyReturns", "Div", []any{i, tooManyReturns{}}, "too many return values"},
		{"WrongType", "Div", []any{i, float64s{}}, "got Generator[float64]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := reflect.PointerTo(reflection.Type[divModWorkload]())
			r := newTestRegistrar(t, w)
			err := r.registerGenerators(test.method, test.generators...)
			if err == nil {
				t.Fatal("unexpected success")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("Error does not contain %q:\n%s", test.want, err.Error())
			}

		})
	}
}

func TestMissingRegisterGenerators(t *testing.T) {
	// Forget to call registerGenerators on some of a workload's methods.
	w := reflect.PointerTo(reflection.Type[divModWorkload]())
	r := newTestRegistrar(t, w)
	err := r.finalize()
	if err == nil {
		t.Fatal("unexpected success")
	}
	const want = "no generators registered for method DivMod"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("Error does not contain %q:\n%s", want, err.Error())
	}
}

// newTestRegistrar returns a new registrar for the provided type.
func newTestRegistrar(t *testing.T, w reflect.Type) *registrar {
	methods := map[string]reflect.Method{}
	for i := 0; i < w.NumMethod(); i++ {
		m := w.Method(i)
		if m.Name != "Init" {
			methods[m.Name] = m
		}
	}

	regsByIntf := map[reflect.Type]*codegen.Registration{}
	for _, reg := range codegen.Registered() {
		regsByIntf[reg.Iface] = reg
	}

	return newRegistrar(t, w, methods, regsByIntf)
}
