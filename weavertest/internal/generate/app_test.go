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

package generate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/weavertest"
)

// TODO(mwhittaker): Induce an error in the encoding, decoding, and RPC call.
func TestErrors(t *testing.T) {
	ctx := context.Background()
	weavertest.Multi.Test(t, func(t *testing.T, client testApp) {
		// Trigger an application error. Verify that an application error
		// is returned.
		x, err := client.Get(ctx, "foo", appError)
		if err == nil || !strings.Contains(err.Error(), "key foo not found") {
			t.Fatalf("expected an application error; got: %v", err)
		}
		if x != 42 {
			t.Fatalf("client.Get: got %d, want 42", x)
		}

		// Do a normal get operation. Verify that the operation succeeds.
		x, err = client.Get(ctx, "foo", noError)
		if err != nil {
			t.Fatal(err)
		}
		if x != 42 {
			t.Fatalf("client.Get: got %d, want 42", x)
		}

		// Check custom error.
		_, err = client.Get(ctx, "custom", customError)
		if err == nil {
			t.Fatal(err)
		}
		var c customErrorValue
		if !errors.As(err, &c) {
			t.Errorf("did not get customError, got error %v of type %T", err, err)
		} else if c.key != "custom" {
			t.Errorf("customError contained wrong key %q, expecting %q", c.key, "custom")
		}

		// Trigger a panic.
		_, err = client.Get(ctx, "foo", panicError)
		if err == nil || !errors.Is(err, weaver.RemoteCallError) {
			t.Fatalf("expected a weaver.RemoteCallError; got: %v", err)
		}
	})
}

func TestPointers(t *testing.T) {
	for _, runner := range weavertest.AllRunners() {
		ctx := context.Background()
		runner.Test(t, func(t *testing.T, client testApp) {
			// Check pointer passing for nil pointer.
			got, err := client.IncPointer(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got != nil {
				t.Fatalf("unexpected non-nil result: %v", got)
			}

			// Check pointer passing for non-nil pointer.
			x := 7
			got, err = client.IncPointer(ctx, &x)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("unexpected nil result")
			}
			if *got != x+1 {
				t.Fatalf("unexpected pointer to %d; expecting %d", *got, x+1)
			}
		})
	}
}
