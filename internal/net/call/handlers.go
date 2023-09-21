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

package call

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

// MethodKey identifies a particular method on a component (formed by
// fingerprinting the component and method name).
type MethodKey [16]byte

// MakeMethodKey returns the fingerprint for the specified method on component.
func MakeMethodKey(component, method string) MethodKey {
	sig := sha256.Sum256([]byte(component + "." + method))
	var fp MethodKey
	copy(fp[:], sig[:])
	return fp
}

// Handler is a function that handles remote procedure calls. Regular
// application errors should be serialized in the returned bytes. A Handler
// should only return a non-nil error if the handler was not able to execute
// successfully.
type Handler func(ctx context.Context, args []byte) ([]byte, error)

// HandlerMap is a mapping from MethodID to a Handler. The zero value for a
// HandlerMap is an empty map.
type HandlerMap struct {
	handlers map[MethodKey]Handler
	names    map[MethodKey]string
}

// NewHandlerMap returns a handler map to which the server handlers can
// be added. A "ready" handler is automatically registered in the new
// returned map.
func NewHandlerMap() *HandlerMap {
	hm := &HandlerMap{
		handlers: map[MethodKey]Handler{},
		names:    map[MethodKey]string{},
	}
	// Add a dummy "ready" handler. Clients will repeatedly call this
	// RPC until it responds successfully, ensuring the server is ready.
	hm.Set("", "ready", func(context.Context, []byte) ([]byte, error) {
		return nil, nil
	})
	return hm
}

// Set registers a handler for the specified method of component.
func (hm *HandlerMap) Set(component, method string, handler Handler) {
	fp := MakeMethodKey(component, method)
	hm.handlers[fp] = handler
	hm.names[fp] = component + "." + method
}

// AddHandlers adds handlers for all methods of the component with the
// specified name. The handlers invoke methods on the specified impl.
func (hm *HandlerMap) AddHandlers(name string, impl any) error {
	reg, ok := codegen.Find(name)
	if !ok {
		return fmt.Errorf("component %s not found", name)
	}
	addLoad := func(uint64, float64) {} // We ignore load updates for now.
	serverStub := reg.ServerStubFn(impl, addLoad)
	for i, n := 0, reg.Iface.NumMethod(); i < n; i++ {
		mname := reg.Iface.Method(i).Name
		handler := serverStub.GetStubFn(mname)
		hm.Set(reg.Name, mname, handler)
	}
	return nil
}
