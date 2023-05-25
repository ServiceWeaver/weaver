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

// Package private is an internal holder of state that different
// Service Weaver packages share without exposing it externally. E.g.,
// if module X wants to expose a function to module Y without making
// it public, X can store the function in a variable in private and Y
// can read it from there.
package private

import (
	"context"
	"reflect"
)

// RunOptions controls a Service  Weaver application execution.
type RunOptions struct {
	// Fakes holds a mapping from component interface type to the fake
	// implementation to use for that component.
	Fakes map[reflect.Type]any
}

// Run runs a Service Weaver application rooted at a component with
// interface type rootType, app as the body of the application, and
// the supplied options.
var Run func(ctx context.Context, rootType reflect.Type, options RunOptions, app func(context.Context, any) error) error

// Get fetches the component with type t with root recorded as the requester.
var Get func(root any, t reflect.Type) (any, error)
