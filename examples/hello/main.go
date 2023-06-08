// Copyright 2022 Google LLC
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
	"fmt"
	"log"
	"net/http"

	"github.com/ServiceWeaver/weaver"
)

func main() {
	if err := weaver.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

//go:generate ../../cmd/weaver/weaver generate

type app struct {
	weaver.Implements[weaver.Main]
	reverser weaver.Ref[Reverser]
	hello    weaver.Listener
}

func (app *app) Main(ctx context.Context) error {
	fmt.Printf("hello listener available on %v\n", app.hello)

	// Serve the /hello endpoint.
	http.Handle("/hello", weaver.InstrumentHandlerFunc("hello",
		func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query().Get("name")
			if name == "" {
				name = "World"
			}
			reversed, err := app.reverser.Get().Reverse(ctx, name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, "Hello, %s!\n", reversed)
		}))
	return http.Serve(app.hello, nil)
}
