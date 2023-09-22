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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/reflection"
)

var positive = Filter(NonNegativeInt(), func(x int) bool { return x != 0 })

type divModWorkload struct {
	divmod weaver.Ref[divMod]
	div    weaver.Ref[div]
	mod    weaver.Ref[mod]
}

func (d *divModWorkload) Init(r Registrar) error {
	r.RegisterGenerators("DivMod", NonNegativeInt(), positive)
	r.RegisterGenerators("Div", NonNegativeInt(), positive)
	r.RegisterGenerators("Mod", NonNegativeInt(), positive)
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

func TestPassingSimulations(t *testing.T) {
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
			r := s.Run(1 * time.Second)
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
	result, err := s.newExecutor().execute(context.Background(), hyperparameters{
		NumReplicas: 1,
		NumOps:      1000,
		FailureRate: 0,
		YieldRate:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.err != nil {
		t.Fatal(result.err)
	}

	byValueCalls := 0
	byPointerCalls := 0
	for _, event := range result.history {
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
	r.RegisterGenerators("DivMod", Range(0, 10), Range(0, 10))
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
	if r.Err == nil {
		t.Fatal("Unexpected success")
	}
}

func TestValidateValidWorkload(t *testing.T) {
	// Call validateWorkload on a valid workload.
	w := reflection.Type[*divModWorkload]()
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
	w := reflection.Type[*invalidWorkload]()
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
	w := reflection.Type[*emptyWorkload]()
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
			if err := validateWorkload(w); err == nil {
				t.Errorf("unexpected success")
			}
		})
	}
}
