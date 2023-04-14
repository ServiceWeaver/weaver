// Copyright 2022 Google LLC
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

package codegen

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"go.opentelemetry.io/otel/trace"
)

// globalRegistry is the global registry used by Register and Registered.
var globalRegistry registry

// Register registers a Service Weaver component.
func Register(reg Registration) {
	if err := globalRegistry.register(reg); err != nil {
		panic(err)
	}
}

// Registered returns the components registered with Register.
func Registered() []*Registration {
	return globalRegistry.allComponents()
}

// registry is a repository for registered Service Weaver components.
// Entries are typically added to the default registry by calls
// to Register in init functions in code generated by "weaver generate".
type registry struct {
	m          sync.Mutex
	components map[reflect.Type]*Registration // the set of registered components, by their interface types
	byName     map[string]*Registration       // map from full component name to registration
}

// Registration is the configuration needed to register a Service Weaver component.
type Registration struct {
	Name     string             // full package-prefixed component name
	Iface    reflect.Type       // interface type for the component
	New      func() any         // returns a new instance of the implementation type
	ConfigFn func(impl any) any // returns pointer to config field in local impl if non-nil
	Routed   bool               // True if calls to this component should be routed

	// Functions that return different types of stubs.
	LocalStubFn  func(impl any, tracer trace.Tracer) any
	ClientStubFn func(stub Stub, caller string) any
	ServerStubFn func(impl any, load func(key uint64, load float64)) Server
}

// register registers a Service Weaver component. If the registry's close method was
// previously called, Register will fail and return a non-nil error.
func (r *registry) register(reg Registration) error {
	if err := verifyRegistration(reg); err != nil {
		return fmt.Errorf("Register(%q): %w", reg.Name, err)
	}

	r.m.Lock()
	defer r.m.Unlock()
	if old, ok := r.components[reg.Iface]; ok {
		x := old.New()
		y := reg.New()
		return fmt.Errorf("component %s already registered for type %T when registering %T",
			reg.Name, x, y)
	}
	if r.components == nil {
		r.components = map[reflect.Type]*Registration{}
	}
	if r.byName == nil {
		r.byName = map[string]*Registration{}
	}
	ptr := &reg
	r.components[reg.Iface] = ptr
	r.byName[reg.Name] = ptr
	return nil
}

func verifyRegistration(reg Registration) error {
	if reg.New == nil {
		return errors.New("nil New")
	}
	if reg.LocalStubFn == nil {
		return errors.New("nil LocalStubFn")
	}
	if reg.ClientStubFn == nil {
		return errors.New("nil ClientStubFn")
	}
	if reg.ServerStubFn == nil {
		return errors.New("nil ServerStubFn")
	}
	return nil
}

// allComponents returns all of the registered components, keyed by name.
func (r *registry) allComponents() []*Registration {
	r.m.Lock()
	defer r.m.Unlock()

	components := make([]*Registration, 0, len(r.components))
	for _, info := range r.components {
		components = append(components, info)
	}
	return components
}

func (r *registry) find(path string) (*Registration, bool) {
	r.m.Lock()
	defer r.m.Unlock()
	reg, ok := r.byName[path]
	return reg, ok
}

// ComponentConfigValidator checks that cfg is a valid configuration
// for the component type whose fully qualified name is given by path.
func ComponentConfigValidator(path, cfg string) error {
	info, ok := globalRegistry.find(path)
	if !ok {
		// Not for a known component.
		return nil
	}
	if info.ConfigFn == nil {
		return fmt.Errorf("unexpected configuration for component %v "+
			"that does not support configuration (add a "+
			"weaver.WithConfig[configType] embedded field to %v and run weaver generate again)",
			info.Name, info.Iface)
	}
	objConfig := info.ConfigFn(info.New())
	config := &protos.AppConfig{Sections: map[string]string{path: cfg}}
	if err := runtime.ParseConfigSection(path, "", config.Sections, objConfig); err != nil {
		return fmt.Errorf("%v: bad config: %w", info.Iface, err)
	}
	return nil
}
