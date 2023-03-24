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

package register_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/internal/register"
	"golang.org/x/sync/errgroup"
)

const x = 42

func TestWriteThenRead(t *testing.T) {
	var r register.WriteOnce[int]
	r.Write(x)
	if got, want := r.Read(), x; got != want {
		t.Fatalf("Read: got %v, want %v", got, want)
	}
}

func TestMultipleReads(t *testing.T) {
	var r register.WriteOnce[int]
	r.Write(x)
	for i := 0; i < 10; i++ {
		if got, want := r.Read(), x; got != want {
			t.Fatalf("Read: got %v, want %v", got, want)
		}
	}
}

func TestReadThenWrite(t *testing.T) {
	var r register.WriteOnce[int]
	go func() {
		time.Sleep(10 * time.Millisecond)
		r.Write(x)
	}()
	if got, want := r.Read(), x; got != want {
		t.Fatalf("Read: got %v, want %v", got, want)
	}
}

func TestOneWriterManyReaders(t *testing.T) {
	var r register.WriteOnce[int]
	var group errgroup.Group
	for i := 0; i < 10; i++ {
		group.Go(func() error {
			if got, want := r.Read(), x; got != want {
				return fmt.Errorf("Read: got %v, want %v", got, want)
			}
			return nil
		})
	}
	r.Write(x)
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestTwoWritesPanics(t *testing.T) {
	defer func() { recover() }()
	var r register.WriteOnce[int]
	r.Write(1)
	r.Write(2)
	t.Fatal("second Write unexpectedly succeeded")
}

func TestTryWrite(t *testing.T) {
	var r register.WriteOnce[int]
	if got, want := r.TryWrite(x), true; got != want {
		t.Fatalf("TryWrite: got %v, want %v", got, want)
	}
	if got, want := r.TryWrite(x+1), false; got != want {
		t.Fatalf("TryWrite: got %v, want %v", got, want)
	}
	if got, want := r.Read(), x; got != want {
		t.Fatalf("Read: got %v, want %v", got, want)
	}
}
