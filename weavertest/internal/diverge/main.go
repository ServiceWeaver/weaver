// Copyright 2022 Google LLC
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

// Package main implements a Service Weaver application that demonstrates where
// singleprocess and multiprocess execution diverge.
package main

import (
	"context"
	"errors"
	"fmt"

	weaver "github.com/ServiceWeaver/weaver"
)

//go:generate ../../../cmd/weaver/weaver generate ./...

// constructorErrors demonstrates that we only propagate constructor errors in
// singleprocess mode. In multiprocess mode, if a constructor returns an error,
// the entire process crashes. This causes GetFailer to return a non-nil
// network error.
func constructorErrors(_ context.Context, root weaver.Instance) (bool, error) {
	_, err := weaver.Get[Failer](root)
	return errors.Is(err, ErrFailed), nil
}

// dealising demonstrates that pointers are only de-aliased in multiprocess
// mode.
func dealiasing(ctx context.Context, root weaver.Instance) (bool, error) {
	p, err := weaver.Get[Pointer](root)
	if err != nil {
		return false, err
	}
	pair, err := p.Get(ctx)
	if err != nil {
		return false, err
	}
	return pair.X == pair.Y, nil
}

// customErrors demonstrates that custom Is methods are ignored in multiprocess
// mode.
func customErrors(ctx context.Context, root weaver.Instance) (bool, error) {
	e, err := weaver.Get[Errer](root)
	if err != nil {
		return false, err
	}
	err = e.Err(ctx, 1)
	// Note that errors.Is(err, IntError{1}) is true.
	return errors.Is(err, IntError{2}), nil
}

func main() {
	ctx := context.Background()
	root := weaver.Init(ctx)

	single, err := constructorErrors(ctx, root)
	if err != nil {
		panic(err)
	}
	if single {
		fmt.Println("Constructor errors are returned by generated getters (singleprocess)")
	} else {
		fmt.Println("Constructor errors are NOT returned by generated getters (multiprocess)")
	}

	single, err = dealiasing(ctx, root)
	if err != nil {
		panic(err)
	}
	if single {
		fmt.Println("Pointers are NOT dealiased (singleprocess)")
	} else {
		fmt.Println("Pointers are dealiased (multiprocess)")
	}

	single, err = customErrors(ctx, root)
	if err != nil {
		panic(err)
	}
	if single {
		fmt.Println("Custom Is methods are ignored (singleprocess)")
	} else {
		fmt.Println("Custom Is methods are NOT ignored (multiprocess)")
	}
}
