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
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"sync"

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

var (
	pathMu      sync.Mutex
	pathCounter int64
)

// NewUnixSocketPath returns the path to use for a new Unix domain socket.
// The path exists in dir, which should be a directory entirely owned by this
// process, with a short full path-name, typically created by runtime.NewTempDir().
func NewUnixSocketPath(dir string) string {
	pathMu.Lock()
	defer pathMu.Unlock()

	// Since dir is private to this process, we can generate unique paths by
	// using a simple incrementing counter. We keep the names small since
	// Unix domain socket paths have a short maximum length.
	pathCounter++
	return filepath.Join(dir, fmt.Sprintf("_uds%d", pathCounter))
}
