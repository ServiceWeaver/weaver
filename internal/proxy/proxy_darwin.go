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

//go:build darwin
//+build darwin

package proxy

import (
	"errors"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"

	"github.com/ServiceWeaver/weaver/internal/logtype"
)

// Proxy is an HTTP proxy that forwards traffic to a set of backends.
type Proxy struct {
	logger   logtype.Logger        // logger
	reverse  httputil.ReverseProxy // underlying proxy
	mu       sync.Mutex            // guards backends
	backends []string              // backend addresses
}

// NewProxy returns a new proxy.
func NewProxy(logger logtype.Logger) *Proxy {
	p := &Proxy{logger: logger}
	p.reverse = httputil.ReverseProxy{
		Director: p.director,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			// for non HTTP/2 cases, set a reasonable limit on the
			// setup reasonable defaults so this proxy wound not run out
			// of requesting adddress under high load pressure, e.g examples/hello
			// default value of MaxIdelConnsPerHost is 2, 5 is the minimum value to
			// make requesting address issue disappear
			MaxIdleConnsPerHost: 5,
		},
	}

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
