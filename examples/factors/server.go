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

package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/ServiceWeaver/weaver"
)

type server struct {
	weaver.Implements[weaver.Main]
	factorer weaver.Ref[Factorer]
	lis      weaver.Listener `weaver:"factors"`
}

func serve(ctx context.Context, s *server) error {
	http.Handle("/", weaver.InstrumentHandlerFunc("/", s.handleFactors))
	s.Logger().Info("factors server running", "addr", s.lis)
	return http.Serve(s.lis, nil)
}

// handleFactors handles the /?x=<number> endpoint.
func (s *server) handleFactors(w http.ResponseWriter, r *http.Request) {
	x, err := strconv.Atoi(r.URL.Query().Get("x"))
	if err != nil {
		msg := fmt.Errorf("bad request: %w\nusage: curl %s/?x=<number>", err, r.Host)
		http.Error(w, msg.Error(), http.StatusBadRequest)
		return
	}
	if x <= 0 {
		msg := fmt.Errorf("non-positive x: %d", x)
		http.Error(w, msg.Error(), http.StatusBadRequest)
		return
	}
	factors, err := s.factorer.Get().Factors(r.Context(), x)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintln(w, factors)
}
