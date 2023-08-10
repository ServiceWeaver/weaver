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
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ServiceWeaver/weaver"
)

type server struct {
	weaver.Implements[weaver.Main]
	mux  http.ServeMux
	odd  weaver.Ref[Odd]
	even weaver.Ref[Even]
	lis  weaver.Listener `weaver:"collatz"`
}

func serve(ctx context.Context, s *server) error {
	s.mux.Handle("/", weaver.InstrumentHandlerFunc("collatz", s.handle))
	s.mux.HandleFunc(weaver.HealthzURL, weaver.HealthzHandler)
	s.Logger(ctx).Debug("Collatz service available", "address", s.lis)
	return http.Serve(s.lis, &s.mux)
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

	var builder strings.Builder
	for x != 1 {
		fmt.Fprintf(&builder, "%d\n", x)
		if x%2 == 0 {
			x, err = s.even.Get().Do(r.Context(), x)
		} else {
			x, err = s.odd.Get().Do(r.Context(), x)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	fmt.Fprintf(&builder, "%d\n", x)
	fmt.Fprint(w, builder.String())
}
