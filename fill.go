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
	"unicode"

	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/internal/weaver"
	"golang.org/x/exp/slog"
)

func init() {
	// See internal/weaver/types.go.
	weaver.SetLogger = setLogger
	weaver.FillRefs = fillRefs
	weaver.FillListeners = fillListeners
}

// See internal/weaver/types.go.
func setLogger(v any, logger *slog.Logger) error {
	x, ok := v.(interface{ setLogger(*slog.Logger) })
	if !ok {
		return fmt.Errorf("FillLogger: %T does not implement weaver.Implements", v)
	}
	x.setLogger(logger)
	return nil
}

// See internal/weaver/types.go.
func fillRefs(impl any, get func(reflect.Type) (any, error)) error {
	p := reflect.ValueOf(impl)
	if p.Kind() != reflect.Pointer {
		return fmt.Errorf("FillRefs: %T not a pointer", impl)
	}
	s := p.Elem()
	if s.Kind() != reflect.Struct {
		return fmt.Errorf("FillRefs: %T not a struct pointer", impl)
	}

	for i, n := 0, s.NumField(); i < n; i++ {
		f := s.Field(i)
		if !f.Type().Implements(reflection.Type[interface{ isRef() }]()) {
			continue
		}

		// Sanity check that field type structure matches weaver.Ref[T].
		if f.Kind() != reflect.Struct {
			panic(fmt.Errorf("FillRefs: weaver.Ref %v is not a struct", f))
		}
		if f.NumField() != 1 {
			panic(fmt.Errorf("FillRefs: weaver.Ref %v does not have one field", f))
		}
		if f.Type().Field(0).Name != "value" {
			panic(fmt.Errorf("FillRefs: weaver.Ref %v field 0 not named %q", f, "value"))
		}

		// Set the component.
		valueField := f.Field(0)
		component, err := get(valueField.Type())
		if err != nil {
			return fmt.Errorf("FillRefs: setting field %v.%s: %w", s.Type(), s.Type().Field(i).Name, err)
		}
		setPossiblyUnexported(valueField, reflect.ValueOf(component))
	}
	return nil
}

// See internal/weaver/types.go.
func fillListeners(impl any, get func(name string) (net.Listener, string, error)) error {
	p := reflect.ValueOf(impl)
	if p.Kind() != reflect.Pointer {
		return fmt.Errorf("FillListeners: %T not a pointer", impl)
	}
	s := p.Elem()
	if s.Kind() != reflect.Struct {
		return fmt.Errorf("FillListeners: %T not a struct pointer", impl)
	}

	for i, n := 0, s.NumField(); i < n; i++ {
		f := s.Field(i)
		t := s.Type().Field(i)
		if f.Type() != reflection.Type[Listener]() {
			continue
		}

		// The listener's name is the field name, unless a tag is present.
		name := t.Name
		if tag := t.Tag.Get("weaver"); tag != "" {
			if !isValidListenerName(tag) {
				return fmt.Errorf("FillListeners: listener tag %s is not a valid Go identifier", tag)
			}
			name = tag
		}

		// Get the listener.
		lis, proxyAddr, err := get(name)
		if err != nil {
			return fmt.Errorf("FillListener: setting field %v.%s: %w", s.Type(), t.Name, err)
		}

		// Set the listener. We have to use UnsafePointer because the field may
		// not be exported.
		l := (*Listener)(f.Addr().UnsafePointer())
		l.Listener = lis
		l.proxyAddr = proxyAddr
	}
	return nil
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

// setPossiblyUnexported sets dst to value. It is equivalent to
// reflect.Value.Set except that it works even if dst was accessed via
// unexported fields.
func setPossiblyUnexported(dst, value reflect.Value) {
	// Use UnsafePointer + NewAt so we can handle unexported fields.
	dst = reflect.NewAt(dst.Type(), dst.Addr().UnsafePointer()).Elem()
	dst.Set(value)
}
