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
	"net"
	"reflect"
	"strings"

	"github.com/ServiceWeaver/weaver/internal/reflection"
)

// fillListeners initializes Listener fields in a component implementation struct.
//   - impl should be a pointer to the implementation struct
//   - get should be a function that returns the required Listener values,
//     namely the network listener and the proxy address.
func fillListeners(impl any, get func(field string) (net.Listener, string, error)) error {
	p := reflect.ValueOf(impl)
	if p.Kind() != reflect.Pointer {
		return fmt.Errorf("not a pointer")
	}
	s := p.Elem()
	if s.Kind() != reflect.Struct {
		return fmt.Errorf("not a struct pointer")
	}
	isListener := reflection.Type[interface{ isListener() }]()
	for i, n := 0, s.NumField(); i < n; i++ {
		// Handle field with type weaver.Listener.
		ref := s.Field(i)
		if !ref.Type().Implements(isListener) {
			continue
		}

		// Sanity check that field type structure matches weaver.Listener.
		if ref.Kind() != reflect.Struct {
			continue
		}
		if ref.NumField() != 2 {
			continue
		}
		if ref.Type().Field(0).Name != "Listener" {
			continue
		}
		if ref.Type().Field(1).Name != "proxyAddr" {
			continue
		}

		// Listener name is a field name, unless a tag is present.
		lisName := strings.ToLower(s.Type().Field(i).Name)
		if tag := s.Type().Field(i).Tag.Get("weaver"); tag != "" {
			lisName = strings.ToLower(tag)
		}
		listener, proxyAddr, err := get(lisName)
		if err != nil {
			return fmt.Errorf("setting field %v.%s: %w", s.Type(), s.Type().Field(i).Name, err)
		}
		setPossiblyUnexported(ref.Field(0), reflect.ValueOf(listener))
		setPossiblyUnexported(ref.Field(1), reflect.ValueOf(proxyAddr))
	}
	return nil
}
