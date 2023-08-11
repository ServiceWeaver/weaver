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

package proxy

import (
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"sync"
)

// Proxy is an HTTP proxy that forwards traffic to a set of backends.
type Proxy struct {
	logger   *slog.Logger          // logger
	reverse  httputil.ReverseProxy // underlying proxy
	mu       sync.Mutex            // guards backends
	backends []string              // backend addresses
}

// NewProxy returns a new proxy.
func NewProxy(logger *slog.Logger) *Proxy {
	p := &Proxy{logger: logger}
	p.reverse = httputil.ReverseProxy{Director: p.director}
	return p
}

// ServeHTTP implements the http.Handler interface.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.reverse.ServeHTTP(w, r)
}

// AddBackend adds a backend to the proxy. Note that backends cannot be
// removed.
func (p *Proxy) AddBackend(backend string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.backends = append(p.backends, backend)
}

// director implements a ReverseProxy.Director function [1].
//
// [1]: https://pkg.go.dev/net/http/httputil#ReverseProxy
func (p *Proxy) director(r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.backends) == 0 {
		p.logger.Error("director", errors.New("no backends"), "url", r.URL)
		return
	}
	r.URL.Scheme = "http" // TODO(mwhittaker): Support HTTPS.
	r.URL.Host = p.backends[rand.Intn(len(p.backends))]
}
