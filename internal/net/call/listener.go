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

package call

import (
	"net"
)

// Listener allows the server to accept RPCs. The set of handlers to use
// for an accepted connection are determined at connection time and may
// be influenced like policies based on an allowed communication graph.
type Listener interface {
	Accept() (net.Conn, *HandlerMap, error)
	Close() error
	Addr() net.Addr
}

// FixedListener returns a listener that allows all calls to supplied
// components. FixedListener is intended for local system services and access
// control happens at the net.Listener level (e.g., by using Unix domain
// sockets).
//
// Note that this differs from the normal listener used for application
// components (that listener only allows component A to call component B if A
// has an edge to B in the component graph).
//
// Each components map entry has the full component name as the key, and the
// component implementation as the value.
func FixedListener(lis net.Listener, components map[string]any) (Listener, error) {
	// Precompute the handler map.
	handlers := NewHandlerMap()
	for path, impl := range components {
		if err := handlers.addHandlers(path, impl); err != nil {
			return nil, err
		}
	}
	return &fixedListener{lis, handlers}, nil
}

type fixedListener struct {
	net.Listener
	handlers *HandlerMap
}

func (f *fixedListener) Accept() (net.Conn, *HandlerMap, error) {
	c, err := f.Listener.Accept()
	return c, f.handlers, err
}
