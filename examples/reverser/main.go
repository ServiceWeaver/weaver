// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/ServiceWeaver/weaver"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

//go:generate ../../cmd/weaver/weaver generate

var (
	address = flag.String("address", "localhost:9000", "Reverser server local address")

	//go:embed index.html
	indexHtml string // index.html served on "/"
)

func main() {
	// Initialize the Service Weaver application.
	flag.Parse()
	root := weaver.Init(context.Background())

	// Get a client to the Reverser component.
	reverser, err := weaver.Get[Reverser](root)
	if err != nil {
		log.Fatal(err)
	}

	// Get a network listener.
	opts := weaver.ListenerOptions{LocalAddress: *address}
	lis, err := root.Listener("reverser", opts)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("hello listener available on %v\n", lis)

	// Serve HTTP traffic.
	var mux http.ServeMux
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, indexHtml)
	})
	mux.HandleFunc("/reverse", func(w http.ResponseWriter, r *http.Request) {
		reversed, err := reverser.Reverse(r.Context(), r.URL.Query().Get("s"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintln(w, reversed)
	})
	handler := weaver.InstrumentHandler("reverser", &mux)
	handler = otelhttp.NewHandler(handler, "http")
	http.Serve(lis, handler)
}
