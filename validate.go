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
	"unicode"

	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"golang.org/x/exp/slices"
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
			switch {
			case f.Type.Implements(reflection.Type[interface{ isRef() }]()):
				// f is a weaver.Ref[T].
				v := f.Type.Field(0) // a Ref[T]'s value field
				if _, ok := intfs[v.Type]; !ok {
					// T is not a registered component interface.
					err := fmt.Errorf(
						"component implementation struct %v has component reference field %v, but component %v was not registered; maybe you forgot to run 'weaver generate'",
						reg.Impl, f.Type, v.Type,
					)
					errs = append(errs, err)
				}

			case f.Type == reflection.Type[Listener]():
				// f is a weaver.Listener.
				name := f.Name
				if tag, ok := f.Tag.Lookup("weaver"); ok {
					if !isValidListenerName(tag) {
						err := fmt.Errorf("component implementation struct %v has invalid listener tag %q", reg.Impl, tag)
						errs = append(errs, err)
						continue
					}
					name = tag
				}
				if !slices.Contains(reg.Listeners, name) {
					err := fmt.Errorf("component implementation struct %v has a listener field %v, but listener %v hasn't been registered; maybe you forgot to run 'weaver generate'", reg.Impl, name, name)
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}

// isValidListenerName returns whether the provided name is a valid
// weaver.Listener name.
func isValidListenerName(name string) bool {
	// We allow valid Go identifiers [1]. This code is taken from [2].
	//
	// [1]: https://go.dev/ref/spec#Identifiers
	// [2]: https://cs.opensource.google/go/go/+/refs/tags/go1.20.6:src/go/token/token.go;l=331-341;drc=19309779ac5e2f5a2fd3cbb34421dafb2855ac21
	if name == "" {
		return false
	}
	for i, c := range name {
		if !unicode.IsLetter(c) && c != '_' && (i == 0 || !unicode.IsDigit(c)) {
			return false
		}
	}
	return true
}
