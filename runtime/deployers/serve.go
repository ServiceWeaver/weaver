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

// Deployers provides useful utilities for implementing Service Weaver deployers.
package deployers

import (
	"context"
	"log/slog"
	"net"

	"github.com/ServiceWeaver/weaver/internal/net/call"
)

// ServeComponents handles method calls made to the specified listener for the specified
// components.
//
// Each components map entry has the full component name as the key, and the component
// implementation as the value.
func ServeComponents(ctx context.Context, listener net.Listener, logger *slog.Logger, components map[string]any) error {
	// Precompute the handler map.
	handlers := call.NewHandlerMap()
	for path, impl := range components {
		if err := handlers.AddHandlers(path, impl); err != nil {
			return err
		}
	}
	f := &fixedListener{listener, handlers}
	return call.Serve(ctx, f, call.ServerOptions{Logger: logger})
}

type fixedListener struct {
	net.Listener
	handlers *call.HandlerMap
}

var _ call.Listener = &fixedListener{}

// Accept implements call.Listener.Accept.
func (f *fixedListener) Accept() (net.Conn, *call.HandlerMap, error) {
	c, err := f.Listener.Accept()
	return c, f.handlers, err
}
