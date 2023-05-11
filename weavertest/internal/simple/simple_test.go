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

package simple_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/weavertest"
	"github.com/ServiceWeaver/weaver/weavertest/internal/simple"
	"github.com/google/uuid"
)

func TestOneComponent(t *testing.T) {
	for _, single := range []bool{true, false} {
		opt := weavertest.Options{SingleProcess: single}
		t.Run(fmt.Sprintf("Single=%t", single), func(t *testing.T) {
			weavertest.Run(t, opt, func(dst simple.Destination) {
				// Get the PID of the dst component. Check whether root and dst are running
				// in the same process.
				cPid := os.Getpid()
				dstPid, _ := dst.Getpid(context.Background())
				sameProcess := cPid == dstPid

				if single && !sameProcess {
					t.Fatal("the root and the dst components should run in the same process")
				}
				if !single && sameProcess {
					t.Fatal("the root and the dst components should run in different processes")
				}
			})
		})
	}
}

func TestTwoComponents(t *testing.T) {
	// Add a list of items to a component (dst) from another component (src). Verify that
	// dst updates the state accordingly.
	type testCase struct {
		name   string
		single bool
		config string
	}
	ctx := context.Background()
	for _, c := range []testCase{
		{"single", true, ""},
		{"multi", false, ""},
		{"colocate", false, `
			[serviceweaver]
			colocate = [
			  [
			    "github.com/ServiceWeaver/weaver/weavertest/internal/simple/Source",
			    "github.com/ServiceWeaver/weaver/weavertest/internal/simple/Destination",
			  ]
		]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			opts := weavertest.Options{
				SingleProcess: c.single,
				Config:        c.config,
			}
			weavertest.Run(t, opts, func(src simple.Source, dst simple.Destination) {
				file := filepath.Join(t.TempDir(), fmt.Sprintf("simple_%s", uuid.New().String()))
				want := []string{"a", "b", "c", "d", "e"}
				for _, in := range want {
					if err := src.Emit(ctx, file, in); err != nil {
						t.Fatal(err)
					}
				}

				got, err := dst.GetAll(ctx, file)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(want, got) {
					t.Fatalf("GetAll() = %v; expecting %v", got, want)
				}
			})
		})
	}
}

func TestServer(t *testing.T) {
	for _, single := range []bool{true, false} {
		t.Run(fmt.Sprintf("Single=%t", single), func(t *testing.T) {
			weavertest.Run(t, weavertest.Options{SingleProcess: single}, func(srv simple.Server) {
				ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second*10)
				defer cancelFunc()
				defer func() {
					err := srv.Shutdown(ctx)
					if err != nil {
						t.Fatalf("Shutdown failed: %v", err)
					}
				}()

				// Check listener properties.
				addr, err := srv.Address(ctx)
				if err != nil {
					t.Fatalf("Could not fetch server address: %v", err)
				}
				if !strings.Contains(addr, ":") {
					t.Fatalf("Bad address %q", addr)
				}
				proxy, err := srv.ProxyAddress(ctx)
				if err != nil {
					t.Fatalf("Could not fetch proxy address: %v", err)
				}
				if single && proxy != "" {
					t.Fatalf("Unexpected proxy %q", proxy)
				}

				// Check server handler.
				url := fmt.Sprintf("http://%s/test", addr)
				t.Logf("Calling %s", url)
				resp, err := http.Get(url)
				if err != nil {
					t.Fatalf("Calling server: %v", err)
				}
				defer resp.Body.Close()
				data, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Reading server response: %v", err)
				}
				if want, got := simple.ServerTestResponse, string(data); got != want {
					t.Fatalf("Wrong response %q, expecting %q", got, want)
				}
			})
		})
	}
}

func TestRoutedCall(t *testing.T) {
	// Make a call to a routed method.
	type testCase struct {
		name   string
		single bool
	}
	ctx := context.Background()
	for _, c := range []testCase{
		{"single", true},
		{"multi", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), fmt.Sprintf("simple_%s", uuid.New().String()))
			weavertest.Run(t, weavertest.Options{SingleProcess: c.single}, func(dst simple.Destination) {
				if err := dst.RoutedRecord(ctx, file, "hello"); err != nil {
					t.Fatal(err)
				}

				want := []string{"routed: hello"}

				got, err := dst.GetAll(ctx, file)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(want, got) {
					t.Fatalf("GetAll() = %v; expecting %v", got, want)
				}
			})
		})
	}
}

func BenchmarkCall(b *testing.B) {
	for _, single := range []bool{true, false} {
		opt := weavertest.Options{SingleProcess: single}
		b.Run(fmt.Sprintf("Single=%t", single), func(b *testing.B) {
			weavertest.Run(b, opt, func(dst simple.Destination) {
				for i := 0; i < b.N; i++ {
					_, err := dst.Getpid(context.Background())
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
