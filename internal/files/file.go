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

// Package files contains file-related utilities.
package files

import (
	"fmt"
	"os"
	"path/filepath"
)

// Writer writes a sequence of bytes to a file. In case of errors, the old file
// contents remain untouched since the writes are applied atomically when the
// Writer is closed.
type Writer struct {
	dst     string   // Name of destination file
	tmp     *os.File // Temporary file to which data is written.
	tmpName string   // Name of temporary file.
	err     error
}

// NewWriter returns a writer that writes to the named files.
//
// The caller should eventually call Cleanup. A recommended pattern is:
//
//	w := files.NewWriter(dst)
//	defer w.Cleanup()
//	... write to w ...
//	err := w.Close()
func NewWriter(file string) *Writer {
	w := &Writer{dst: file}
	dir, base := filepath.Dir(file), filepath.Base(file)
	w.tmp, w.err = os.CreateTemp(dir, base+".tmp*")
	if w.err == nil {
		w.tmpName = w.tmp.Name()
	}
	return w
}

// Write writes p and returns the number of bytes written, which will either be
// len(p), or the returned error will be non-nil.
func (w *Writer) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.tmp == nil {
		return 0, fmt.Errorf("%s: already cleaned up", w.dst)
	}
	n, err := w.tmp.Write(p)
	if err != nil {
		w.err = err
		w.Cleanup()
	}
	return n, err
}

// Close saves the written bytes to the destination file and closes the writer.
// Close returns an error if any errors were encountered, including during earlier
// Write calls.
func (w *Writer) Close() error {
	if w.err != nil {
		return w.err
	}
	if w.tmp == nil {
		return fmt.Errorf("%s: already cleaned up", w.dst)
	}
	err := w.tmp.Close()
	w.tmp = nil
	if err != nil {
		os.Remove(w.tmpName)
		return err
	}
	if err := os.Rename(w.tmpName, w.dst); err != nil {
		os.Remove(w.tmpName)
		return err
	}
	return nil
}

// Cleanup releases any resources in use by the writer, without attempting to write
// collected bytes to the destination. It is safe to call Cleanup() even if Close or
// Cleanup have already been called.
func (w *Writer) Cleanup() {
	if w.tmp == nil {
		return
	}
	w.tmp.Close()
	w.tmp = nil
	os.Remove(w.tmpName)
}

// DefaultDataDir returns the default directory for Service Weaver files:
// $XDG_DATA_HOME/serviceweaver, or ~/.local/share/serviceweaver if
// XDG_DATA_HOME is not set.
func DefaultDataDir() (string, error) {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		// Default to ~/.local/share
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataDir = filepath.Join(home, ".local", "share")
	}
	regDir := filepath.Join(dataDir, "serviceweaver")
	if err := os.MkdirAll(regDir, 0700); err != nil {
		return "", err
	}

	return regDir, nil
}
