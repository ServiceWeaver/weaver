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

// Package testdeployer includes unit tests for a remote weavelet spawned by a
// test deployer. Thest tests are in a separate package from the remote
// weavelet code to avoid a cyclic dependency.
package testdeployer

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

//go:generate ../../cmd/weaver/weaver generate .

// Define three components---a, b, and c---that are used in unit tests.

type a interface {
	A(context.Context, int) (int, error)
}

type b interface {
	B(context.Context, int) (int, error)
}

type c interface {
	C(context.Context, int) (int, error)
}

type aimpl struct {
	weaver.Implements[a]
	lis weaver.Listener //lint:ignore U1000 used in remoteweavelet_test.go
	b   weaver.Ref[b]
}

type bimpl struct {
	weaver.Implements[b]
	c weaver.Ref[c]
}

type cimpl struct {
	weaver.Implements[c]
}

func (a *aimpl) A(ctx context.Context, x int) (int, error) {
	a.Logger(ctx).Debug("A")
	return a.b.Get().B(ctx, x)
}

func (b *bimpl) B(ctx context.Context, x int) (int, error) {
	b.Logger(ctx).Debug("B")
	return b.c.Get().C(ctx, x)
}

func (c *cimpl) C(ctx context.Context, x int) (int, error) {
	c.Logger(ctx).Debug("C")
	return x, nil
}
