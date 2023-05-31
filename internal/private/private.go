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

// AppOptions controls a Service Weaver application execution.
type AppOptions struct {
	// Fakes holds a mapping from component interface type to the fake
	// implementation to use for that component.
	Fakes map[reflect.Type]any
}

// Starts starts a Service Weaver application.
// Callers are required to call app.Wait().
var Start func(ctx context.Context, options AppOptions) (App, error)

// App is an internal handle to a Service Weaver application.
type App interface {
	// Wait returns when the application has ended.
	Wait(context.Context) error

	// Get fetches the component with type t from wlet.
	Get(requester string, t reflect.Type) (any, error)

	// ListenerAddress returns the address (host:port) of the
	// named listener, waiting for the listener to be created
	// if necessary.
	ListenerAddress(name string) (string, error)
}
