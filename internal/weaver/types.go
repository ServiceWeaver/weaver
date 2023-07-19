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
	"net"
	"reflect"

	"golang.org/x/exp/slog"
)

var (
	// The following values are populated by an init function in the main
	// weaver package. The circuitousness is to avoid cyclic dependencies.

	// SetLogger sets the logger of a component implementation struct. impl
	// should be a pointer to the implementation struct.
	SetLogger func(impl any, logger *slog.Logger) error

	// FillRefs initializes Ref[T] fields in a component implement struct.
	//   - impl should be a pointer to the implementation struct
	//   - get should be a function that returns the component of interface
	//     type T when passed the reflect.Type for T.
	FillRefs func(impl any, get func(reflect.Type) (any, error)) error

	// FillListeners initializes Listener fields in a component implementation
	// struct.
	//   - impl should be a pointer to the implementation struct
	//   - get should be a function that returns the required Listener values,
	//     namely the network listener and the proxy address.
	FillListeners func(impl any, get func(string) (net.Listener, string, error)) error
)
