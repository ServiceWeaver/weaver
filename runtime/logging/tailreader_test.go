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

package logging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	sync "sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// lockedBuffer is a thread-safe bytes.Buffer.
type lockedBuffer struct {
	mu  sync.Mutex
	buf *bytes.Buffer
}

// Read implements the io.Reader interface.
func (b *lockedBuffer) Read(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Read(p)
}

// Write implements the io.Writer interface.
func (b *lockedBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// tailWriter is the io.Writer returned by tailpipe.
type tailWriter struct {
	buf   *lockedBuffer
	ready chan struct{}
}

// Write implements the io.Writer interface.
func (w *tailWriter) Write(p []byte) (int, error) {
	n, err := w.buf.Write(p)
	if n > 0 {
		select {
		case w.ready <- struct{}{}:
		default:
		}
	}
	return n, err
}

// tailpipe, like io.Pipe and net.Pipe, returns a connected pair of a
// tailReader and tailWriter. Any writes to the tailWriter can be read from the
// tailReader.
func tailpipe(ctx context.Context) (*tailReader, *tailWriter) {
	buf := lockedBuffer{buf: bytes.NewBuffer(nil)}
	ready := make(chan struct{})
	wait := func() error {
		select {
		case <-ready:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	reader := newTailReader(&buf, wait)
	writer := &tailWriter{buf: &buf, ready: ready}
	return reader, writer
}

// polltail, like tailpipe, returns a connected pair of a reader and a writer.
// The tailReader returned by polltail, however, polls when blocked.
func polltail(ctx context.Context) (*tailReader, *lockedBuffer) {
	writer := &lockedBuffer{buf: bytes.NewBuffer(nil)}
	wait := func() error {
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-timer.C:
			return nil
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
	reader := newTailReader(writer, wait)
	return reader, writer
}

func TestTailReader(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	piper, pipew := tailpipe(ctx)
	pollr, pollw := polltail(ctx)

	for _, test := range []struct {
		name string
		r    io.Reader
		w    io.Writer
	}{
		{"Pipe", piper, pipew},
		{"Poll", pollr, pollw},
	} {
		t.Run(test.name, func(t *testing.T) {
			read := func(p []byte, want []byte) {
				n, err := test.r.Read(p)
				if err != nil {
					t.Fatalf("r.Read: %v", err)
				}
				if diff := cmp.Diff(want, p[:n]); diff != "" {
					t.Fatalf("r.Read (-want +got):\n%s", diff)
				}
			}

			// Test simple reads and writes.
			p := make([]byte, 4)
			test.w.Write([]byte{1, 2, 3})
			read(p, []byte{1, 2, 3})
			test.w.Write([]byte{4, 5, 6, 7, 8, 9})
			read(p, []byte{4, 5, 6, 7})
			read(p, []byte{8, 9})
			go func() {
				time.Sleep(10 * time.Millisecond)
				test.w.Write([]byte{10, 11, 12, 13, 14, 15})
			}()
			read(p, []byte{10, 11, 12, 13})
			read(p, []byte{14, 15})

			// Test io.ReadFull.
			go func() {
				time.Sleep(10 * time.Millisecond)
				test.w.Write([]byte{16, 17})
				time.Sleep(10 * time.Millisecond)
				test.w.Write([]byte{18, 19, 20})
				time.Sleep(10 * time.Millisecond)
				test.w.Write([]byte{21, 22})
			}()
			p = make([]byte, 6)
			_, err := io.ReadFull(test.r, p)
			if err != nil {
				t.Fatalf("io.ReadAll: %v", err)
			}
			if diff := cmp.Diff([]byte{16, 17, 18, 19, 20, 21}, p); diff != "" {
				t.Fatalf("io.ReadAll (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTailReaderError(t *testing.T) {
	var buf bytes.Buffer
	want := fmt.Errorf("error")
	r := newTailReader(&buf, func() error { return want })
	p := make([]byte, 4)
	if _, err := r.Read(p); !errors.Is(err, want) {
		t.Fatalf("r.Read: want %v, got %v", want, err)
	}
}
