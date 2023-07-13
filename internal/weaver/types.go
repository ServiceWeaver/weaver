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

import "reflect"

var (
	// IsImplements, IsRef, and IsListener return whether the provided type is
	// a weaver.Implements[T], weaver.Ref[T], and weaver.Listener respectively.
	// These values are populated by an init function in the main weaver
	// package. The circuitousness is to avoid cyclic dependencies.
	//
	// TODO(mwhittaker): Improve these abstractions to initialize values as
	// well. The code in fillrefs, filllisteners, etc. relies on some unsafe
	// reflection stuff.
	IsImplements func(reflect.Type) bool
	IsRef        func(reflect.Type) bool
	IsListener   func(reflect.Type) bool
)
