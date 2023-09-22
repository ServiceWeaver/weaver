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

	"github.com/ServiceWeaver/weaver"
)

//go:generate ../../cmd/weaver/weaver generate .

// This file contains components used to test the simulator in sim_test.go. The
// callgraph is intentionally overcomplicated to exercise the simulator.

// Component interfaces.

type divMod interface {
	// DivMod(n, d) returns n/d, n%d.
	DivMod(context.Context, int, int) (int, int, error)
}

type div interface {
	// Div(n, d) returns n/d.
	Div(context.Context, int, int) (int, error)
}

type mod interface {
	// Mod(n, d) returns n%d.
	Mod(context.Context, int, int) (int, error)
}

type identity interface {
	// Identity(x) returns x.
	Identity(context.Context, int) (int, error)
}

type blocker interface {
	// Block blocks until the provided context is cancelled.
	Block(context.Context) error
}

type panicker interface {
	// Panic panics if the provided bool is true.
	Panic(context.Context, bool) error
}

// Component implementation structs.

type divModImpl struct {
	weaver.Implements[divMod]
	div weaver.Ref[div]
	mod weaver.Ref[mod]
}

type divImpl struct {
	weaver.Implements[div]
	identity weaver.Ref[identity]
}

type modImpl struct {
	weaver.Implements[mod]
	identity weaver.Ref[identity]
}

type identityImpl struct {
	weaver.Implements[identity]
}

type blockerImpl struct {
	weaver.Implements[blocker]
}

type panickerImpl struct {
	weaver.Implements[panicker]
}

// Component implementations.

func (i *divModImpl) DivMod(ctx context.Context, n, d int) (int, int, error) {
	div, err := i.div.Get().Div(ctx, n, d)
	if err != nil {
		return 0, 0, err
	}
	mod, err := i.mod.Get().Mod(ctx, n, d)
	if err != nil {
		return 0, 0, err
	}
	return div, mod, nil
}

func (i *divImpl) Div(ctx context.Context, n, d int) (int, error) {
	if d == 0 {
		return 0, zeroError{}
	}
	n, err := i.identity.Get().Identity(ctx, n)
	if err != nil {
		return 0, err
	}
	d, err = i.identity.Get().Identity(ctx, d)
	if err != nil {
		return 0, err
	}
	return n / d, nil
}

func (i *modImpl) Mod(ctx context.Context, n, d int) (int, error) {
	if d == 0 {
		return 0, zeroError{}
	}
	n, err := i.identity.Get().Identity(ctx, n)
	if err != nil {
		return 0, err
	}
	d, err = i.identity.Get().Identity(ctx, d)
	if err != nil {
		return 0, err
	}
	return n % d, nil
}

func (i *identityImpl) Identity(ctx context.Context, x int) (int, error) {
	return x, nil
}

func (*blockerImpl) Block(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (*panickerImpl) Panic(_ context.Context, b bool) error {
	if b {
		panic(fmt.Errorf("Panic!"))
	}
	return nil
}

// Errors.

type zeroError struct {
	weaver.AutoMarshal
}

func (zeroError) Error() string {
	return "zero"
}
