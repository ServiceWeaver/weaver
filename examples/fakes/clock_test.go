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

package fakes

import (
	"context"
	"testing"

	"github.com/ServiceWeaver/weaver/weavertest"
)

// fakeClock is a fake implementation of the Clock component that always
// returns a fixed time.
type fakeClock struct {
	now int64 // the time returned by UnixMicro
}

// UnixMicro implements the Clock interface.
func (f *fakeClock) UnixMicro(context.Context) (int64, error) {
	return f.now, nil
}

func TestClock(t *testing.T) {
	for _, runner := range weavertest.AllRunners() {
		// Register a fake Clock implementation with the runner.
		fake := &fakeClock{100}
		runner.Fakes = append(runner.Fakes, weavertest.Fake[Clock](fake))

		// When a fake is registered for a component, all instances of that
		// component dispatch to the fake.
		runner.Test(t, func(t *testing.T, clock Clock) {
			now, err := clock.UnixMicro(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if now != 100 {
				t.Fatalf("bad time: got %d, want %d", now, 100)
			}

			fake.now = 200
			now, err = clock.UnixMicro(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if now != 200 {
				t.Fatalf("bad time: got %d, want %d", now, 200)
			}
		})
	}
}
