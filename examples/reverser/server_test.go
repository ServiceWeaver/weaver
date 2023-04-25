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
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/ServiceWeaver/weaver/weavertest"
)

func TestServer(t *testing.T) {
	weavertest.Run(t, weavertest.Options{}, func(reverser Reverser) {
		lis, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			t.Fatal(err)
		}
		defer lis.Close()

		s := &server{reverser: reverser, lis: lis}
		go serve(context.Background(), s)

		url := fmt.Sprintf("http://%s/reverse?s=foo", lis.Addr().String())
		reply, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		bytes, err := io.ReadAll(reply.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(bytes), "oof"; got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	})
}
