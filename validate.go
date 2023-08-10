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
	"errors"
	"fmt"
	"reflect"

	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

// validateRegistrations validates the provided registrations, returning an
// diagnostic error if they are invalid. Note that some validation is performed
// by 'weaver generate', but because users can run a Service Weaver app after
// forgetting to run 'weaver generate', some checks have to be done at runtime.
func validateRegistrations(regs []*codegen.Registration) error {
	// Gather the set of registered interfaces.
	intfs := map[reflect.Type]struct{}{}
	for _, reg := range regs {
		intfs[reg.Iface] = struct{}{}
	}

	// Check that for every weaver.Ref[T] field in a component implementation
	// struct, T is a registered interface.
	var errs []error
	for _, reg := range regs {
		for i := 0; i < reg.Impl.NumField(); i++ {
			f := reg.Impl.Field(i)
			if !f.Type.Implements(reflection.Type[interface{ isRef() }]()) {
				// f is not a Ref[T].
				continue
			}
			v := f.Type.Field(0) // a Ref[T]'s value field
			if _, ok := intfs[v.Type]; !ok {
				// T is not a registered component interface.
				err := fmt.Errorf(
					"component implementation struct %v has field %v, but component %v was not registered; maybe you forgot to run 'weaver generate'",
					reg.Impl, f.Type, v.Type,
				)
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
