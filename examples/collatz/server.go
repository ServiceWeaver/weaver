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

package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	weaver "github.com/ServiceWeaver/weaver"
)

type server struct {
	mux  http.ServeMux
	root weaver.Instance
	odd  Odd
	even Even
}

func newServer(root weaver.Instance) (*server, error) {
	odd, err := weaver.Get[Odd](root)
	if err != nil {
		return nil, err
	}
	even, err := weaver.Get[Even](root)
	if err != nil {
		return nil, err
	}
	s := &server{root: root, odd: odd, even: even}
	s.mux.Handle("/", weaver.InstrumentHandler("collatz", http.HandlerFunc(s.handle)))
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {})
	return s, nil
}

func (s *server) Run() error {
	lis, err := s.root.Listener("collatz", weaver.ListenerOptions{LocalAddress: *localAddr})
	if err != nil {
		return err
	}
	s.root.Logger().Debug("Collatz service available", "address", lis)
	return http.Serve(lis, otelhttp.NewHandler(&s.mux, "http"))
}

func (s *server) handle(w http.ResponseWriter, r *http.Request) {
	x, err := strconv.Atoi(r.URL.Query().Get("x"))
	if err != nil {
		http.Error(w, fmt.Sprintf("error: %v; usage: curl localhost:port/?x=<number>", err), http.StatusBadRequest)
		return
	}

	if x <= 0 {
		http.Error(w, fmt.Sprintf("%d is not positive", x), http.StatusBadRequest)
		return
	}

	// The collatz sequence is not in the cache. Compute the sequence, and
	// store it in the cache.
	var builder strings.Builder
	for x != 1 {
		fmt.Fprintf(&builder, "%d\n", x)
		if x%2 == 0 {
			x, err = s.even.Do(r.Context(), x)
		} else {
			x, err = s.odd.Do(r.Context(), x)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	fmt.Fprintf(&builder, "%d\n", x)
	fmt.Fprint(w, builder.String())
}
