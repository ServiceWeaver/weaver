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
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/ServiceWeaver/weaver"
)

//go:generate ../../cmd/weaver/weaver generate

var (
	address = flag.String("address", "localhost:9000", "Wrap server local address")

	//go:embed index.html
	indexHtml string // index.html served on "/"

	//go:embed wrap.js
	wrapJs string // wrap.js script served on "/wrap.js"

	//go:embed style.css
	styleCss string // style.css stylesheet served on "/style.css"
)

func main() {
	// Initialize the Service Weaver application.
	flag.Parse()
	ctx := context.Background()
	root := weaver.Init(ctx)

	// Get a client to the Wrapper component.
	wrapper, err := weaver.Get[Wrapper](root)
	if err != nil {
		log.Fatal(err)
	}

	// Get a network listener.
	opts := weaver.ListenerOptions{LocalAddress: *address}
	lis, err := root.Listener("wrap", opts)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrap server available on %v\n", lis)

	// Serve HTTP traffic.
	var mux http.ServeMux
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, indexHtml)
	})
	mux.HandleFunc("/wrap.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		fmt.Fprint(w, wrapJs)
	})
	mux.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		fmt.Fprint(w, styleCss)
	})
	mux.HandleFunc("/wrap", func(w http.ResponseWriter, r *http.Request) {
		// TODO(mwhittaker): Take n as a parameter.
		wrapped, err := wrapper.Wrap(r.Context(), r.URL.Query().Get("s"), 80)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, wrapped)
	})
	handler := weaver.InstrumentHandler("hello", &mux)
	http.Serve(lis, handler)
}
