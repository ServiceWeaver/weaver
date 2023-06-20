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

// Package chain defines a chain of components A -> B -> C with methods to
// propagate a value from the head of the chain to the tail of the chain.
package chain

import (
	"context"
	"sync"

	"github.com/ServiceWeaver/weaver"
)

//go:generate ../../../cmd/weaver/weaver generate

type A interface {
	Propagate(context.Context, int) error
}

type B interface {
	Propagate(context.Context, int) error
}

type C interface {
	Propagate(context.Context, int) error
}

type a struct {
	weaver.Implements[A]
	b   weaver.Ref[B]
	mu  sync.Mutex
	val int
}

type b struct {
	weaver.Implements[B]
	c   weaver.Ref[C]
	mu  sync.Mutex
	val int
}

type c struct {
	weaver.Implements[C]
	mu  sync.Mutex
	val int
}

func (a *a) Propagate(ctx context.Context, val int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.val = val
	return a.b.Get().Propagate(ctx, val+1)
}

func (b *b) Propagate(ctx context.Context, val int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.val = val
	return b.c.Get().Propagate(ctx, val+1)
}

func (c *c) Propagate(_ context.Context, val int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.val = val
	return nil
}
