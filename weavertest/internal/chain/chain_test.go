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

package chain

import (
	"context"
	"testing"

	"github.com/ServiceWeaver/weaver/weavertest"
)

func TestOneComponentImpl(t *testing.T) {
	// Tests weaver.Test with a component implementation pointer argument.
	for _, runner := range weavertest.AllRunners() {
		runner.Test(t, func(t *testing.T, a A, c *c) {
			if err := a.Propagate(context.Background(), 1); err != nil {
				t.Fatal(err)
			}
			if got, want := c.val, 3; got != want {
				t.Fatalf("got %d, want %d", got, want)
			}
		})
	}
}

func BenchOneComponentImpl(b *testing.B) {
	// Tests weaver.Bench with a component implementation pointer argument.
	for _, runner := range weavertest.AllRunners() {
		runner.Bench(b, func(b *testing.B, a A, c *c) {
			if err := a.Propagate(context.Background(), 1); err != nil {
				b.Fatal(err)
			}
			if got, want := c.val, 3; got != want {
				b.Fatalf("got %d, want %d", got, want)
			}
		})
	}
}

func TestTwoComponentImpls(t *testing.T) {
	// Tests weaver.Test with two component implementation pointer arguments.
	for _, runner := range weavertest.AllRunners() {
		runner.Test(t, func(t *testing.T, a A, b *b, c *c) {
			if err := a.Propagate(context.Background(), 1); err != nil {
				t.Fatal(err)
			}
			if got, want := b.val, 2; got != want {
				t.Fatalf("got %d, want %d", got, want)
			}
			if got, want := c.val, 3; got != want {
				t.Fatalf("got %d, want %d", got, want)
			}
		})
	}
}

func TestDuplicateComponentImpl(t *testing.T) {
	// Tests weaver.Test with two component implementation pointer arguments
	// of type *d. Both pointers should point to the same underlying instance
	// of d.
	for _, runner := range weavertest.AllRunners() {
		runner.Test(t, func(t *testing.T, a A, c1 *c, c2 *c) {
			if err := a.Propagate(context.Background(), 1); err != nil {
				t.Fatal(err)
			}
			if got, want := c1.val, 3; got != want {
				t.Fatalf("got %d, want %d", got, want)
			}
			if got, want := c2.val, 3; got != want {
				t.Fatalf("got %d, want %d", got, want)
			}
		})
	}
}

func TestOverlappingComponentInterfaceAndImpl(t *testing.T) {
	// Tests weaver.Test with an A argument and an *a argument. The underlying
	// implementation of the A argument should be the *a argument.
	for _, runner := range weavertest.AllRunners() {
		runner.Test(t, func(t *testing.T, intf A, impl *a) {
			if err := intf.Propagate(context.Background(), 1); err != nil {
				t.Fatal(err)
			}
			if got, want := impl.val, 1; got != want {
				t.Fatalf("got %d, want %d", got, want)
			}
		})
	}
}

func TestOverlappingComponentInterfaceAndFake(t *testing.T) {
	// Tests weaver.Test with an A argument and fake A implementation. The
	// underlying implementation of the A argument should be the fake.
	for _, runner := range weavertest.AllRunners() {
		fake := &fakea{}
		runner.Fakes = append(runner.Fakes, weavertest.Fake[A](fake))
		runner.Test(t, func(t *testing.T, a A) {
			if err := a.Propagate(context.Background(), 1); err != nil {
				t.Fatal(err)
			}
			if got, want := fake.val, 1; got != want {
				t.Fatalf("got %d, want %d", got, want)
			}
		})
	}
}

type fakea struct{ val int }

func (a *fakea) Propagate(_ context.Context, val int) error {
	a.val = val
	return nil
}
