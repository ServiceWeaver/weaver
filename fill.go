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
	"log/slog"
	"net"
	"reflect"

	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/internal/weaver"
)

func init() {
	// See internal/weaver/types.go.
	weaver.SetLogger = setLogger
	weaver.HasRefs = hasRefs
	weaver.FillRefs = fillRefs
	weaver.HasListeners = hasListeners
	weaver.FillListeners = fillListeners
	weaver.HasConfig = hasConfig
	weaver.GetConfig = getConfig
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
func hasRefs(impl any) bool {
	p := reflect.ValueOf(impl)
	if p.Kind() != reflect.Pointer {
		return false
	}
	s := p.Elem()
	if s.Kind() != reflect.Struct {
		return false
	}

	for i, n := 0, s.NumField(); i < n; i++ {
		f := s.Field(i)
		if !f.CanAddr() {
			continue
		}
		p := reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Interface()
		if _, ok := p.(interface{ isRef() }); ok {
			return true
		}
	}
	return false
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
		if !f.CanAddr() {
			continue
		}
		p := reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Interface()
		x, ok := p.(interface{ setRef(any) })
		if !ok {
			continue
		}

		// Set the component.
		valueField := f.Field(0)
		component, err := get(valueField.Type())
		if err != nil {
			return fmt.Errorf("FillRefs: setting field %v.%s: %w", s.Type(), s.Type().Field(i).Name, err)
		}
		x.setRef(component)
	}
	return nil
}

// See internal/weaver/types.go.
func hasListeners(impl any) bool {
	p := reflect.ValueOf(impl)
	if p.Kind() != reflect.Pointer {
		return false
	}
	s := p.Elem()
	if s.Kind() != reflect.Struct {
		return false
	}

	for i, n := 0, s.NumField(); i < n; i++ {
		f := s.Field(i)
		if f.Type() == reflection.Type[Listener]() {
			return true
		}
	}
	return false
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
		if tag, ok := t.Tag.Lookup("weaver"); ok {
			if !isValidListenerName(name) {
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

// See internal/weaver/types.go.
func hasConfig(impl any) bool {
	_, ok := impl.(interface{ getConfig() any })
	return ok
}

// See internal/weaver/types.go.
func getConfig(impl any) any {
	if c, ok := impl.(interface{ getConfig() any }); ok {
		return c.getConfig()
	}
	return nil
}
