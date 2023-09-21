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

package weaver

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/go-cmp/cmp"
)

func TestLoggerSend(t *testing.T) {
	type testCase struct {
		name       string        // Test name
		msgs       []string      // Messages to send
		maxBatches int           // Maximum number of batches to expect
		delay      time.Duration // Articificial delay per received batch
	}
	makeList := func(n int) []string {
		list := make([]string, n)
		for i := range list {
			list[i] = fmt.Sprint(i)
		}
		return list
	}

	for _, c := range []testCase{
		{"single", []string{"hello"}, 1, 0},
		{"multiple", makeList(4), 4, 0},
		{"many", makeList(10000), 100, time.Millisecond},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Send a message and make sure it is passed to dst.
			want := c.msgs
			var got []string
			var batches atomic.Int32
			var wait sync.WaitGroup
			wait.Add(len(want))
			rl := newRemoteLogger(os.Stderr)
			go rl.run(ctx, func(ctx context.Context, batch *protos.LogEntryBatch) error {
				batches.Add(1)
				for _, e := range batch.Entries {
					got = append(got, e.Msg)
					wait.Done()
				}
				time.Sleep(c.delay)
				return nil
			})
			for _, s := range want {
				rl.log(&protos.LogEntry{Msg: s})
			}
			wait.Wait()
			if b := batches.Load(); int(b) > c.maxBatches {
				t.Errorf("received too many batches %d, expecting at most %d", b, c.maxBatches)
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("log delivery error: (-want,+got):\n%s\n", diff)
			}
		})
	}
}

func TestLoggerError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fallback := collector(make(chan string))
	rl := newRemoteLogger(fallback)
	go rl.run(ctx, func(ctx context.Context, batch *protos.LogEntryBatch) error {
		return fmt.Errorf("fake error")
	})
	rl.log(&protos.LogEntry{Msg: "hello"})
	str := <-fallback
	for _, expect := range []string{"hello", "serviceweaver/logerror", "fake error"} {
		if !strings.Contains(str, expect) {
			t.Errorf("fallback output %q did not contain expected %q", str, expect)
		}
	}
}

// collector is a thread-safe destination for log messages.
type collector chan string

func (c collector) Write(data []byte) (int, error) {
	c <- string(data)
	return len(data), nil
}
