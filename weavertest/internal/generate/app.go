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

// Package generate tests that "weaver generate"-generated code *executes*
// correctly. This is in contrast to the tests in the main "generate" package,
// which only test that the generated code *compiles* correctly.
package generate

import (
	"context"
	"fmt"

	weaver "github.com/ServiceWeaver/weaver"
)

//go:generate ../../../cmd/weaver/weaver generate

type behaviorType int

const (
	appError behaviorType = iota
	panicError
	noError
)

type testApp interface {
	Get(_ context.Context, key string, behavior behaviorType) (int, error)
	IncPointer(_ context.Context, arg *int) (*int, error)
}

type impl struct {
	weaver.Implements[testApp]
}

// Get returns an error or a value based on the expected behavior.
func (p *impl) Get(_ context.Context, key string, behavior behaviorType) (int, error) {
	switch behavior {
	case appError:
		return 42, fmt.Errorf("key %v not found in the store", key)
	case panicError:
		panic("panic")
	case noError:
		return 42, nil
	}
	return 42, fmt.Errorf("unknown behavior type: %v", behavior)
}

// IncPointer passes a value by pointer and increments it.
func (p *impl) IncPointer(_ context.Context, arg *int) (*int, error) {
	if arg == nil {
		return nil, nil
	}
	res := new(int)
	*res = *arg + 1
	return res, nil
}
