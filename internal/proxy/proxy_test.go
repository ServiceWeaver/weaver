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

func TestProxyNoBackend(t *testing.T) {
	proxy := NewProxy(slog.Default())

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

func TestProxyOneBackend(t *testing.T) {
	backendResponse := "hello"
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(backendResponse))
	}))
	defer backendServer.Close()

	backendUrl, err := url.Parse(backendServer.URL)
	if err != nil {
		t.Fatal(err)
	}

	proxy := NewProxy(slog.Default())
	proxy.AddBackend(backendUrl.Host)

	frontend := httptest.NewServer(proxy)
	defer frontend.Close()

	resp, err := http.Get(frontend.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := string(b), backendResponse; g != e {
		t.Errorf("got body %q; expected %q", g, e)
	}
}

func TestProxyConcurrentAddBackend(t *testing.T) {
	const numBackends = 10
	hello := "hello"
	backendServers := []*httptest.Server{}
	for i := 0; i < numBackends; i++ {
		backendResponse := fmt.Sprintf("%s:%d", hello, i)
		backendServers = append(backendServers, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(backendResponse))
		})))
	}

	backendUrls := make([]*url.URL, 0, len(backendServers))
	for _, bs := range backendServers {
		backendUrl, err := url.Parse(bs.URL)
		if err != nil {
			t.Fatal(err)
		}
		backendUrls = append(backendUrls, backendUrl)
	}

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

	frontend := httptest.NewServer(proxy)
	defer frontend.Close()

	res, err := http.Get(frontend.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	for _, bs := range backendServers {
		bs.Close()
	}

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
