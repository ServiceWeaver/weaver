// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package proxy

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// TestProxyNoBackend checks the behavior of the proxy when there are no
// backend servers added.
// It expects a 502 Bad Gateway response when making a request through the proxy
func TestProxyNoBackend(t *testing.T) {
	proxy := NewProxy(slog.Default())

	// No backend was added to proxy
	frontend := httptest.NewServer(proxy)
	defer frontend.Close()

	res, err := http.Get(frontend.URL)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadGateway {
		t.Errorf("request to bad proxy = %v; want 502 StatusBadGateway", res.Status)
	}
}

// TestProxyOneBackend verifies the behavior of the proxy when a
// single backend server is added.
// It sends a request through the proxy and checks if the response matches
// the expected backend response.
func TestProxyOneBackend(t *testing.T) {
	backendResponse := "hello"
	// Create a single backend server
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(backendResponse))
	}))
	defer backendServer.Close()

	backendUrl, err := url.Parse(backendServer.URL)
	if err != nil {
		t.Fatal(err)
	}

	// Create a proxy and add the backend url
	proxy := NewProxy(slog.Default())
	proxy.AddBackend(backendUrl.Host)

	frontend := httptest.NewServer(proxy)
	defer frontend.Close()

	// Make client request
	resp, err := http.Get(frontend.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// Expect body response matches the backend response
	if g, e := string(b), backendResponse; g != e {
		t.Errorf("got body %q; expected %q", g, e)
	}
}

// TestProxyConcurrentAddBackend assesses the proxy's ability to handle
// concurrent addition of multiple backend servers.
// It creates multiple backend servers, adds them to the proxy concurrently,
// and then sends a request through the proxy.
// It checks if the response matches one of the expected backend responses.
func TestProxyConcurrentAddBackend(t *testing.T) {
	const numBackends = 10 // number backends added to proxy for test
	hello := "hello"
	backendServers := []*httptest.Server{}

	// Create a list of backend servers
	for i := 0; i < numBackends; i++ {
		backendResponse := fmt.Sprintf("%s:%d", hello, i) // backend response format
		backendServers = append(backendServers, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(backendResponse))
		})))
	}

	// Parse backend urls
	backendUrls := make([]*url.URL, 0, len(backendServers))
	for _, bs := range backendServers {
		backendUrl, err := url.Parse(bs.URL)
		if err != nil {
			t.Fatal(err)
		}
		backendUrls = append(backendUrls, backendUrl)
	}

	// Create a proxy and add list backend servers concurently to proxy
	proxy := NewProxy(slog.Default())
	var wg sync.WaitGroup
	wg.Add(len(backendUrls))
	for i := 0; i < len(backendUrls); i++ {
		go func(i int) {
			defer wg.Done()
			proxy.AddBackend(backendUrls[i].Host)
		}(i)
	}
	wg.Wait()

	// Create a fontend server
	frontend := httptest.NewServer(proxy)
	defer frontend.Close()

	// Make client request
	res, err := http.Get(frontend.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	// Read body
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	// Close all backend servers
	for _, bs := range backendServers {
		bs.Close()
	}

	// Ensure a body response matches with one of backend responses
	matched := false
	for i := 0; i < numBackends; i++ {
		format := fmt.Sprintf("%s:%d", hello, i)
		if format == string(b) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("unexpected response body got: %s", string(b))
	}
}
