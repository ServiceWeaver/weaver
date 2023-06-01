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
	"fmt"
	"reflect"

	"github.com/ServiceWeaver/weaver/internal/reflection"
)

// fillRefs initializes Ref[T] fields in a component implement struct.
//   - impl should be a pointer to the implementation struct
//   - get should be a function that returns the component of interface type
//     T when passed the reflect.Type for T.
func fillRefs(impl any, get func(reflect.Type) (any, error)) error {
	p := reflect.ValueOf(impl)
	if p.Kind() != reflect.Pointer {
		return fmt.Errorf("not a pointer")
	}
	s := p.Elem()
	if s.Kind() != reflect.Struct {
		return fmt.Errorf("not a struct pointer")
	}
	isRef := reflection.Type[interface{ isRef() }]()
	for i, n := 0, s.NumField(); i < n; i++ {
		// Handle field with type weaver.Ref[T].
		ref := s.Field(i)
		if !ref.Type().Implements(isRef) {
			continue
		}
		// Sanity check that field type structure matches weaver.Ref[T].
		if ref.Kind() != reflect.Struct {
			continue // XXX Panic?
		}
		if ref.NumField() != 1 {
			continue // XXX Panic?
		}
		if ref.Type().Field(0).Name != "value" {
			continue // XXX Panic?
		}
		valueField := ref.Field(0)
		component, err := get(valueField.Type())
		if err != nil {
			return fmt.Errorf("setting field %v.%s: %w", s.Type(), s.Type().Field(i).Name, err)
		}
		setPossiblyUnexported(valueField, reflect.ValueOf(component))
	}
	return nil
}

// setPossiblyUnexported sets dst to value. It is equivalent to reflect.Value.Set
// except that it works even if dst was accessed via unexported fields.
func setPossiblyUnexported(dst, value reflect.Value) {
	// Use UnsafePointer + NewAt so we can handle unexported fields.
	dst = reflect.NewAt(dst.Type(), dst.Addr().UnsafePointer()).Elem()
	dst.Set(value)
}
