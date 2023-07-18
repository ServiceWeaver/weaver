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
	"reflect"
)

// A Weavelet is an agent that hosts a set of components.
type Weavelet interface {
	// GetIntf returns a handle to the component with the provided interface
	// type. For example, given component interface Foo, GetImpl(Foo) returns a
	// value of type Foo.
	GetIntf(t reflect.Type) (any, error)

	// GetImpl returns the component implementation with the provided type. If
	// the component does not exist, it is created. For example, given
	// component interface Foo and implementing struct foo, GetImpl(foo)
	// returns an instance of type *foo.
	GetImpl(t reflect.Type) (any, error)
}
