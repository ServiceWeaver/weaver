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
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

// newTestRegistrar[T] returns a new registrar for workload type T.
func newTestRegistrar[T Workload](t *testing.T) *registrar {
	registered := map[reflect.Type]struct{}{}
	for _, reg := range codegen.Registered() {
		registered[reg.Iface] = struct{}{}
	}
	return newRegistrar(t, reflection.Type[T](), registered)
}

func TestDuplicateRegisterGeneratorsCalls(t *testing.T) {
	// Call registerGenerators twice for the same method.
	r := newTestRegistrar[*divModWorkload](t)
	if err := r.registerGenerators("DivMod", NonNegativeInt(), positive); err != nil {
		t.Fatal(err)
	}
	err := r.registerGenerators("DivMod", NonNegativeInt(), positive)
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
	i, p := NonNegativeInt(), positive
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
		{"WrongType", "Div", []any{i, Float64()}, "got Generator[float64]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := newTestRegistrar[*divModWorkload](t)
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
	r := newTestRegistrar[*divModWorkload](t)
	err := r.finalize()
	if err == nil {
		t.Fatal("unexpected success")
	}
	const want = "no generators registered for method"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("Error does not contain %q:\n%s", want, err.Error())
	}
}

type gen1 struct{}
type gen2 struct{}

func (gen1) Generate(*rand.Rand) int { return 1 }
func (gen2) Generate(*rand.Rand) int { return 2 }

func TestChangeGeneratorType(t *testing.T) {
	// Register a generator of type gen1. Then, reset the registrar and
	// register a generator of type gen2. This should produce an error because
	// generator types cannot change across executions.
	r := newTestRegistrar[*oneCallWorkload](t)
	if err := r.registerGenerators("Foo", gen1{}); err != nil {
		t.Fatal(err)
	}
	if err := r.finalize(); err != nil {
		t.Fatal(err)
	}

	r.reset()
	err := r.registerGenerators("Foo", gen2{})
	if err == nil {
		t.Fatal("unexpected success")
	}
	const want = "but previously had type sim.gen1"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("Error does not contain %q:\n%s", want, err.Error())
	}
}
