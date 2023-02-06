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
	"errors"
	"io"
)

// tailReader is an io.Reader that behaves like tail -f.
type tailReader struct {
	src            io.Reader    // the reader from which we read
	waitForChanges func() error // blocks until src has bytes to read
}

var _ io.Reader = &tailReader{}

// newTailReader returns a reader that yields data from src. On end-of-file, it
// waits for the reader to grow by calling waitForChanges instead of returning
// io.EOF (and exits immediately if waitForChanges returns an error).
func newTailReader(src io.Reader, waitForChanges func() error) *tailReader {
	return &tailReader{
		src:            src,
		waitForChanges: waitForChanges,
	}
}

// Read returns available data, or waits for more data to become available.
// Read implements the io.Reader interface.
func (t *tailReader) Read(p []byte) (int, error) {
	for {
		n, err := t.src.Read(p)
		if !errors.Is(err, io.EOF) {
			return n, err
		}

		if n > 0 {
			// Return what we got.
			return n, nil
		}

		if err := t.waitForChanges(); err != nil {
			return 0, err
		}
	}
}
