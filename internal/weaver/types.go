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

package weaver

import (
	"log/slog"
	"net"
	"reflect"
)

var (
	// The following values are populated by an init function in the main
	// weaver package. The circuitousness is to avoid cyclic dependencies.

	// SetLogger sets the logger of a component implementation struct. impl
	// should be a pointer to the implementation struct.
	SetLogger func(impl any, logger *slog.Logger) error

	// HasRefs returns whether the provided component implementation has
	// weaver.Refs fields.
	HasRefs func(impl any) bool

	// FillRefs initializes Ref[T] fields in a component implement struct.
	//   - impl should be a pointer to the implementation struct
	//   - get should be a function that returns the component of interface
	//     type T when passed the reflect.Type for T.
	FillRefs func(impl any, get func(reflect.Type) (any, error)) error

	// HasListeners returns whether the provided component implementation has
	// weaver.Listener fields.
	HasListeners func(impl any) bool

	// FillListeners initializes Listener fields in a component implementation
	// struct.
	//   - impl should be a pointer to the implementation struct
	//   - get should be a function that returns the required Listener values,
	//     namely the network listener and the proxy address.
	FillListeners func(impl any, get func(string) (net.Listener, string, error)) error

	// HasConfig returns whether the provided component implementation has
	// an embedded weaver.Config field.
	HasConfig func(impl any) bool

	// GetConfig returns the config stored in the provided component
	// implementation, or returns nil if there is no config.
	GetConfig func(impl any) any
)
