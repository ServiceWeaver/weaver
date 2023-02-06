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

package protomsg

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// discardingLogger implements the logtype.Logger interface. We can't use a
// logging.TestLogger because of cyclic imports.
type discardingLogger struct{}

func (d discardingLogger) Debug(msg string, labels ...any)            {}
func (d discardingLogger) Info(msg string, labels ...any)             {}
func (d discardingLogger) Error(msg string, err error, labels ...any) {}

func TestPanicHandler(t *testing.T) {
	// Run and query server.
	const msg = "zardoz"
	server := httptest.NewServer(panicHandler(
		discardingLogger{},
		func(http.ResponseWriter, *http.Request) {
			panic(msg)
		}),
	)
	resp, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	// Check status code.
	if got, want := resp.StatusCode, http.StatusInternalServerError; got != want {
		t.Fatalf("status code: got %v, want %v", got, want)
	}

	// Check error message.
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(out); !strings.Contains(s, msg) {
		t.Fatalf("message does not contain %q:\n%s", msg, s)
	}
}
