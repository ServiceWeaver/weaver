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

	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/weavertest"
	"github.com/ServiceWeaver/weaver/weavertest/internal/simple"
	"github.com/google/uuid"
)

func TestOneComponent(t *testing.T) {
	for _, runner := range weavertest.AllRunners() {
		runner.Test(t, func(t *testing.T, dst simple.Destination) {
			// Get the PID of the dst component. Check whether root and dst are running
			// in the same process.
			cPid := os.Getpid()
			dstPid, _ := dst.Getpid(context.Background())
			sameProcess := cPid == dstPid
			if runner.Name == weavertest.Multi.Name {
				if sameProcess {
					t.Fatal("the root and the dst components should run in different processes")
				}
			} else {
				if !sameProcess {
					t.Fatal("the root and the dst components should run in the same process")
				}
			}
		})
	}
}

type fakeDest struct{ file, msg string }

func (f *fakeDest) Getpid(context.Context) (int, error)                { return 100, nil }
func (f *fakeDest) GetAll(context.Context, string) ([]string, error)   { return nil, nil }
func (f *fakeDest) RoutedRecord(context.Context, string, string) error { return nil }
func (f *fakeDest) Record(ctx context.Context, file, msg string) error {
	f.file = file
	f.msg = msg
	return nil
}

func TestFake(t *testing.T) {
	for _, runner := range weavertest.AllRunners() {
		fake := &fakeDest{}
		runner.Fakes = append(runner.Fakes, weavertest.Fake[simple.Destination](fake))
		runner.Test(t, func(t *testing.T, src simple.Source) {
			src.Emit(context.Background(), "file", "msg")
			if fake.file != "file" || fake.msg != "msg" {
				t.Fatal("fake Destination method not called")
			}
		})
	}
}

func TestTwoComponents(t *testing.T) {
	// Add a list of items to a component (dst) from another component (src). Verify that
	// dst updates the state accordingly.
	ctx := context.Background()

	// Runner with Source and Destination colocate.
	colocate := weavertest.Multi
	colocate.Name = "Colocate"
	colocate.Config = `
		[serviceweaver]
		colocate = [
		  [
		    "github.com/ServiceWeaver/weaver/weavertest/internal/simple/Source",
		    "github.com/ServiceWeaver/weaver/weavertest/internal/simple/Destination",
		  ]
	]`

	for _, runner := range append(weavertest.AllRunners(), colocate) {
		runner.Test(t, func(t *testing.T, src simple.Source, dst simple.Destination) {
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
	}
}

func TestServer(t *testing.T) {
	for _, runner := range weavertest.AllRunners() {
		runner.Test(t, func(t *testing.T, srv simple.Server) {
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
			if runner.Name != weavertest.Multi.Name && proxy != "" {
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
	}
}

func TestRoutedCall(t *testing.T) {
	// Make a call to a routed method.
	ctx := context.Background()
	for _, runner := range weavertest.AllRunners() {
		runner.Test(t, func(t *testing.T, dst simple.Destination) {
			file := filepath.Join(t.TempDir(), fmt.Sprintf("simple_%s", uuid.New().String()))
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
	}
}

func BenchmarkCall(b *testing.B) {
	for _, runner := range weavertest.AllRunners() {
		runner.Bench(b, func(b *testing.B, dst simple.Destination) {
			ctx := context.Background()
			for i := 0; i < b.N; i++ {
				_, err := dst.Getpid(ctx)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkTracedCall(b *testing.B) {
	weavertest.Local.Bench(b, func(b *testing.B, dst simple.Destination) {
		ctx, span := traceio.TestTracer().Start(context.Background(), "foo")

		// Make a new span every 100 calls.
		const k = 100
		for i := 0; i < b.N; {
			batch := b.N - i
			if batch > k {
				batch = k // Bound trace length
			}
			i += batch
			for j := 0; j < batch; j++ {
				_, err := dst.Getpid(ctx)
				if err != nil {
					b.Fatal(err)
				}
			}
			span.End()
		}
	})
}
