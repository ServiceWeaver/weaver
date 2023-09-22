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
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver"
)

// See TestPassingExecution.
type passingWorkload struct {
	divmod weaver.Ref[divMod]
}

func (p *passingWorkload) Init(r Registrar) error {
	r.RegisterGenerators("DivMod", Range(0, 100), Range(1, 100))
	return nil
}

func (p *passingWorkload) DivMod(ctx context.Context, x, y int) error {
	div, mod, err := p.divmod.Get().DivMod(ctx, x, y)
	if err != nil {
		// Ignore errors.
		return nil
	}
	if div != x/y {
		return fmt.Errorf("div %d/%d: got %d, want %d", x, y, div, x/y)
	}
	if mod != x%y {
		return fmt.Errorf("mod %d%%%d: got %d, want %d", x, y, mod, x%y)
	}
	return nil
}

func TestPassingExecution(t *testing.T) {
	// Run the execution, ignoring any errors. The execution should pass.
	params := hyperparameters{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 0.1,
		YieldRate:   0.5,
	}
	s := New(t, &passingWorkload{}, Options{})
	result, err := s.newExecutor().execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
}

// See TestFailingExecution.
type failingWorkload struct {
	divmod weaver.Ref[divMod]
}

func (f *failingWorkload) Init(r Registrar) error {
	r.RegisterGenerators("DivMod", Range(0, 100), Range(0, 1))
	return nil
}

func (f *failingWorkload) DivMod(ctx context.Context, x, y int) error {
	_, _, err := f.divmod.Get().DivMod(ctx, x, y)
	return err
}

func TestFailingExecution(t *testing.T) {
	// Run the execution, erroring out if we ever have a zero denominator. The
	// execution should fail with extremely high likelihood.
	params := hyperparameters{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 0.1,
		YieldRate:   0.5,
	}
	s := New(t, &failingWorkload{}, Options{})
	result, err := s.newExecutor().execute(context.Background(), params)
	if err == nil && result.err == nil {
		t.Fatal("unexpected success")
	}
}

func TestExecuteGraveyardEntries(t *testing.T) {
	// This test re-runs failed TestUnsuccessfulExecution executions.
	graveyard, err := readGraveyard(filepath.Join("testdata", "sim", "TestUnsuccessfulExecution"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range graveyard {
		params := hyperparameters{
			Seed:        entry.Seed,
			NumReplicas: entry.NumReplicas,
			NumOps:      entry.NumOps,
			FailureRate: entry.FailureRate,
			YieldRate:   entry.YieldRate,
		}
		s := New(t, &failingWorkload{}, Options{})
		result, err := s.newExecutor().execute(context.Background(), params)
		if err == nil && result.err == nil {
			t.Fatal("unexpected success")
		}
	}
}

// See TestPanickingExecution.
type panickingMethodWorkload struct {
	p weaver.Ref[panicker]
}

func (p *panickingMethodWorkload) Init(r Registrar) error {
	r.RegisterGenerators("Panic", Flip(0.1))
	return nil
}

func (p *panickingMethodWorkload) Panic(ctx context.Context, b bool) error {
	p.p.Get().Panic(ctx, b)
	return nil
}

// See TestPanickingxecution.
type panickingOpWorkload struct{}

func (p *panickingOpWorkload) Init(r Registrar) error {
	r.RegisterGenerators("Panic", Flip(0.1))
	return nil
}

func (p *panickingOpWorkload) Panic(ctx context.Context, b bool) error {
	if b {
		panic("Panic!")
	}
	return nil
}

func TestPanickingExecution(t *testing.T) {
	for _, test := range []struct {
		name     string
		workload Workload
	}{
		{"PanickingMethod", &panickingMethodWorkload{}},
		{"PanickingOp", &panickingOpWorkload{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The execution should panic (with extremely high likelihood), but
			// the executor should catch the panic and surface it as an error.
			params := hyperparameters{
				NumReplicas: 2,
				NumOps:      1000,
				FailureRate: 0.1,
				YieldRate:   0.5,
			}
			s := New(t, test.workload, Options{})
			result, err := s.newExecutor().execute(context.Background(), params)
			if err == nil && result.err == nil {
				t.Fatal("unexpected success")
			}
		})
	}
}

// See TestCancelledExecution.
type cancellableWorkload struct {
	b weaver.Ref[blocker]
}

func (c *cancellableWorkload) Init(r Registrar) error {
	r.RegisterGenerators("Block")
	return nil
}

func (c *cancellableWorkload) Block(ctx context.Context) error {
	c.b.Get().Block(ctx)
	return ctx.Err()
}

func TestCancelledExecution(t *testing.T) {
	// Run a blocking execution and cancel it.
	params := hyperparameters{NumReplicas: 10, NumOps: 1000}
	s := New(t, &cancellableWorkload{}, Options{})

	const delay = 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), delay)
	defer cancel()
	errs := make(chan error)
	go func() {
		result, err := s.newExecutor().execute(ctx, params)
		if err != nil {
			errs <- err
		} else {
			errs <- result.err
		}
	}()

	failBefore := time.Now().Add(delay / 2)
	failAfter := time.After(delay * 2)
	select {
	case err := <-errs:
		if !errors.Is(err, ctx.Err()) {
			t.Fatalf("error: got %v, want %v", err, ctx.Err())
		}
		if time.Now().Before(failBefore) {
			t.Fatal("execution cancelled prematurely")
		}
	case <-failAfter:
		t.Fatal("execution not cancelled promptly")
	}
}

// See TestFailureRateZero.
type noFailureWorkload struct {
	divmod weaver.Ref[divMod]
}

func (n *noFailureWorkload) Init(r Registrar) error {
	r.RegisterGenerators("DivMod", Range(0, 100), Range(1, 100))
	return nil
}

func (n *noFailureWorkload) DivMod(ctx context.Context, x, y int) error {
	_, _, err := n.divmod.Get().DivMod(ctx, x, y)
	return err
}

func TestFailureRateZero(t *testing.T) {
	// With FailureRate set to zero, no operation should fail.
	params := hyperparameters{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 0.0,
		YieldRate:   0.5,
	}
	s := New(t, &noFailureWorkload{}, Options{})
	result, err := s.newExecutor().execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
}

// See TestFailureRateOne.
type totalFailureWorkload struct {
	divmod weaver.Ref[divMod]
}

func (t *totalFailureWorkload) Init(r Registrar) error {
	r.RegisterGenerators("DivMod", Range(0, 100), Range(1, 100))
	return nil
}

func (t *totalFailureWorkload) DivMod(ctx context.Context, x, y int) error {
	_, _, err := t.divmod.Get().DivMod(ctx, x, y)
	if err == nil {
		return fmt.Errorf("unexpected success")
	}
	return nil
}

func TestFailureRateOne(t *testing.T) {
	// With FailureRate set to one, all operations should fail.
	params := hyperparameters{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 1.0,
		YieldRate:   0.5,
	}
	s := New(t, &totalFailureWorkload{}, Options{})
	result, err := s.newExecutor().execute(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
}

// See TestInjectedErrors.
type injectedErrorWorkload struct {
	divmod weaver.Ref[divMod]

	mu       sync.Mutex
	nextId   int
	executed map[int]struct{}
	errored  map[int]struct{}
}

func (i *injectedErrorWorkload) Init(r Registrar) error {
	i.executed = map[int]struct{}{}
	i.errored = map[int]struct{}{}
	r.RegisterFake(Fake[divMod](injectedErrorDivMod{i}))
	r.RegisterGenerators("DivMod")
	return nil
}

func (i *injectedErrorWorkload) DivMod(ctx context.Context) error {
	i.mu.Lock()
	id := i.nextId
	i.nextId++
	i.mu.Unlock()

	_, _, err := i.divmod.Get().DivMod(ctx, id, id)
	if errors.Is(err, weaver.RemoteCallError) {
		i.mu.Lock()
		i.errored[id] = struct{}{}
		i.mu.Unlock()
	}

	// TODO(mwhittaker): Allow people to register a postcondition check with a
	// workload.
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.nextId == 1000 {
		failedWithoutExecuting := 0
		failedAfterExecuting := 0
		for id := range i.errored {
			if _, ok := i.executed[id]; ok {
				failedAfterExecuting++
			} else {
				failedWithoutExecuting++
			}
		}
		if failedWithoutExecuting == 0 {
			return fmt.Errorf("no calls failed without executing")
		}
		if failedAfterExecuting == 0 {
			return fmt.Errorf("no calls failed after executing")
		}
	}
	return nil
}

type injectedErrorDivMod struct {
	i *injectedErrorWorkload
}

func (i injectedErrorDivMod) DivMod(_ context.Context, x, _ int) (int, int, error) {
	i.i.mu.Lock()
	defer i.i.mu.Unlock()
	i.i.executed[x] = struct{}{}
	return 0, 0, nil
}

func TestInjectedErrors(t *testing.T) {
	// An injected RemoteCallError can happen before or after a call is
	// executed. Record the calls that error out without executing and the
	// calls that error out after executing. Both should happen.
	params := hyperparameters{
		NumReplicas: 10,
		NumOps:      1000,
		FailureRate: 0.5,
		YieldRate:   0.5,
	}
	s := New(t, &injectedErrorWorkload{}, Options{})
	result, err := s.newExecutor().execute(context.Background(), params)
	if err != nil || result.err != nil {
		t.Fatal(err)
	}
}

// See TestFakes.
type fakeDivMod struct{}

func (fakeDivMod) DivMod(context.Context, int, int) (int, int, error) {
	return 42, 42, nil
}

type fakeWorkload struct {
	divmod weaver.Ref[divMod]
}

func (f *fakeWorkload) Init(r Registrar) error {
	r.RegisterFake(Fake[divMod](fakeDivMod{}))
	r.RegisterGenerators("DivMod", Range(0, 100), Range(1, 100))
	return nil
}

func (f *fakeWorkload) DivMod(ctx context.Context, x, y int) error {
	div, mod, err := f.divmod.Get().DivMod(ctx, x, y)
	if err != nil {
		// Ignore errors.
		return nil
	}
	if div != 42 {
		return fmt.Errorf("div %d/%d: got %d, want %d", x, y, div, 42)
	}
	if mod != 42 {
		return fmt.Errorf("mod %d%%%d: got %d, want %d", x, y, mod, 42)
	}
	return nil
}

func TestFakes(t *testing.T) {
	params := hyperparameters{
		NumReplicas: 10,
		NumOps:      1000,
		YieldRate:   0.5,
	}
	s := New(t, &fakeWorkload{}, Options{})
	result, err := s.newExecutor().execute(context.Background(), params)
	if err != nil || result.err != nil {
		t.Fatal(err)
	}
}

func TestExtractIDs(t *testing.T) {
	const traceID = 42
	const spanID = 9001
	ctx := withIDs(context.Background(), traceID, spanID)
	if got, _ := extractIDs(ctx); got != traceID {
		t.Errorf("trace id: got %d, want %d", got, traceID)
	}
	if _, got := extractIDs(ctx); got != spanID {
		t.Errorf("span id: got %d, want %d", got, spanID)
	}
}

func TestExtractIDsOnInvalidContext(t *testing.T) {
	traceID, spanID := extractIDs(context.Background())
	if traceID != 0 {
		t.Errorf("trace id: got %d, want 0", traceID)
	}
	if spanID != 0 {
		t.Errorf("span id: got %d, want 0", spanID)
	}
}

// See TestBadContextPropagation.
type badContextPropagationWorkload struct {
	id weaver.Ref[identity]
}

func (b *badContextPropagationWorkload) Init(r Registrar) error {
	r.RegisterGenerators("Foo")
	return nil
}

func (b *badContextPropagationWorkload) Foo(context.Context) error {
	// Note that we don't call Identity with provided context.
	b.id.Get().Identity(context.Background(), 42)
	return nil
}

func TestBadContextPropagation(t *testing.T) {
	// Run a workload that doesn't propagate contexts. This is erroneous and
	// should cause the simulation to fail.
	params := hyperparameters{
		NumReplicas: 1,
		NumOps:      1,
		FailureRate: 0.0,
		YieldRate:   0.0,
	}
	s := New(t, &badContextPropagationWorkload{}, Options{})
	result, err := s.newExecutor().execute(context.Background(), params)
	if err == nil && result.err == nil {
		t.Fatal("unexpected success")
	}
}

// A workload with no method calls and no generators.
type noCallsNoGenWorkload struct{}

func (*noCallsNoGenWorkload) Init(r Registrar) error {
	r.RegisterGenerators("Foo")
	return nil
}

func (*noCallsNoGenWorkload) Foo(context.Context) error {
	return nil
}

// A workload with no method calls.
type noCallsWorkload struct{}

func (*noCallsWorkload) Init(r Registrar) error {
	r.RegisterGenerators("Foo", NonNegativeInt())
	return nil
}

func (*noCallsWorkload) Foo(context.Context, int) error {
	return nil
}

// A workload with one method call per op.
type oneCallWorkload struct {
	id weaver.Ref[identity]
}

func (*oneCallWorkload) Init(r Registrar) error {
	r.RegisterGenerators("Foo", NonNegativeInt())
	return nil
}

func (o *oneCallWorkload) Foo(ctx context.Context, x int) error {
	o.id.Get().Identity(ctx, x)
	return nil
}

func BenchmarkNewWorkload(b *testing.B) {
	for _, bench := range []struct {
		name     string
		workload Workload
	}{
		{"NoCallsNoGen", &noCallsNoGenWorkload{}},
		{"NoCalls", &noCallsWorkload{}},
		{"OneCall", &oneCallWorkload{}},
	} {
		b.Run(bench.name, func(b *testing.B) {
			s := New(b, bench.workload, Options{})
			exec := s.newExecutor()
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				workload := reflect.New(exec.w.Elem()).Interface().(Workload)
				exec.registrar.reset()
				if err := workload.Init(exec.registrar); err != nil {
					b.Fatal(err)
				}
				if err := exec.registrar.finalize(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkResetExecutor(b *testing.B) {
	for _, bench := range []struct {
		name     string
		workload Workload
	}{
		{"NoCallsNoGen", &noCallsNoGenWorkload{}},
		{"NoCalls", &noCallsWorkload{}},
		{"OneCall", &oneCallWorkload{}},
	} {
		b.Run(bench.name, func(b *testing.B) {
			s := New(b, bench.workload, Options{})
			exec := s.newExecutor()
			params := hyperparameters{
				NumReplicas: 1,
				NumOps:      1,
				FailureRate: 0,
				YieldRate:   1,
			}
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				workload := reflect.New(exec.w.Elem()).Interface().(Workload)
				exec.registrar.reset()
				if err := workload.Init(exec.registrar); err != nil {
					b.Fatal(err)
				}
				if err := exec.registrar.finalize(); err != nil {
					b.Fatal(err)
				}
				fakes := exec.registrar.fakes
				ops := exec.registrar.ops
				if err := exec.reset(workload, fakes, ops, params); err != nil {
					b.Fatal(err)
				}
			}
		})
	}

}

func BenchmarkExecutions(b *testing.B) {
	for _, bench := range []struct {
		name     string
		workload Workload
	}{
		{"NoCallsNoGen", &noCallsNoGenWorkload{}},
		{"NoCalls", &noCallsWorkload{}},
		{"OneCall", &oneCallWorkload{}},
	} {
		b.Run(bench.name, func(b *testing.B) {
			s := New(b, bench.workload, Options{})
			exec := s.newExecutor()
			params := hyperparameters{
				NumReplicas: 1,
				NumOps:      1,
				FailureRate: 0,
				YieldRate:   1,
			}
			ctx := context.Background()
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result, err := exec.execute(ctx, params)
				if err != nil {
					b.Fatal(err)
				}
				if result.err != nil {
					b.Fatal(result.err)
				}
			}
		})
	}
}

func BenchmarkNumReplicas(b *testing.B) {
	for _, numReplicas := range []int{1, 2, 3} {
		name := fmt.Sprintf("%d-Replicas", numReplicas)
		b.Run(name, func(b *testing.B) {
			s := New(b, &oneCallWorkload{}, Options{})
			exec := s.newExecutor()
			params := hyperparameters{
				NumReplicas: numReplicas,
				NumOps:      1,
				FailureRate: 0,
				YieldRate:   1,
			}
			ctx := context.Background()
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result, err := exec.execute(ctx, params)
				if err != nil {
					b.Fatal(err)
				}
				if result.err != nil {
					b.Fatal(result.err)
				}
			}
		})
	}
}

func BenchmarkParallelExecutions(b *testing.B) {
	for _, bench := range []struct {
		name     string
		workload Workload
	}{
		{"NoCallsNoGen", &noCallsNoGenWorkload{}},
		{"NoCalls", &noCallsWorkload{}},
		{"OneCall", &oneCallWorkload{}},
	} {
		ncpu := runtime.NumCPU()
		for _, p := range []int{1, 10, ncpu, 2 * ncpu, 5 * ncpu, 10 * ncpu} {
			b.Run(fmt.Sprintf("%s/Parallelism-%d", bench.name, p), func(b *testing.B) {
				s := New(b, bench.workload, Options{Parallelism: p})
				r := s.Run(time.Second)
				if r.Err != nil {
					b.Fatal(r.Err)
				}
				b.ReportMetric(float64(r.NumOps)/float64(b.Elapsed().Seconds()), "ops/s")
				b.ReportMetric(float64(r.NumExecutions)/float64(b.Elapsed().Seconds()), "execs/s")
			})
		}
	}
}
