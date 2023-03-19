// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package versioned_test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/internal/versioned"
)

func TestLock(t *testing.T) {
	// Test plan: Start a bunch of go-routines that increment and decrement
	// a counter by the same amount. Make sure that the counter value remains
	// zero before and after increment/decrement, indicating that no concurrent
	// increments occurred.
	v := versioned.Version(atomic.Int64{})
	const num = 200
	for i := 0; i < num; i++ {
		go func() {
			v.Lock()
			defer v.Unlock()
			if c := v.Val.Add(1); c != 1 {
				panic(fmt.Sprint("invalid count, want 1, got", c))
			}
			time.Sleep(time.Millisecond)
			if c := v.Val.Add(-1); c != 0 {
				panic(fmt.Sprint("invalid count, want 0, got", c))
			}
		}()
	}
}

func TestRLock(t *testing.T) {
	// Test plan: Acquire a bunch of concurrent RLocks. Make sure the test
	// completes.
	v := versioned.Version(int(0))
	const num = 200
	for i := 0; i < num; i++ {
		v.RLock("")
	}
	for i := 0; i < num; i++ {
		v.RUnlock()
	}
}

func TestReadWrite(t *testing.T) {
	v := versioned.Version(int(0))
	expect := func(version string, val int) string {
		newVersion := v.RLock(version)
		defer v.RUnlock()
		if v.Val != val {
			t.Fatalf("expected value %v, got %v", val, v.Val)
		}
		return newVersion
	}
	write := func(val int) {
		v.Lock()
		defer v.Unlock()
		v.Val = val
	}
	expect("", 0)
	write(1)
	version := expect("", 1)
	if version == "" {
		t.Fatalf("expected non-empty version, got %q", version)
	}
	write(2)
	newVersion := expect(version, 2)
	if newVersion == "" || newVersion == version {
		t.Fatalf("expected non-empty version different from %q, got %q", version, newVersion)
	}
}
