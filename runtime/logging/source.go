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
	"context"

	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// Source is a log source. You can use a source to cat and follow logs,
// filtering log entries using a query. A Source can safely be used
// concurrently from multiple goroutines.
type Source interface {
	// Query returns a log entry reader.
	//
	// Log entries produced by a single component replica are guaranteed to be
	// returned in increasing timestamp order, but log entries across component
	// replicas are interleaved in timestamp order only on a best-effort basis.
	//
	// If follow is true, then the returned reader follows logs like `tail -f`.
	// Otherwise, log entries that are created during an executing query may or
	// may not be returned. Similarly, the returned reader does not return old
	// log entries that have been garbage collected.
	Query(ctx context.Context, q Query, follow bool) (Reader, error)
}

// Reader is a log entry iterator. A Reader is not thread-safe; its methods
// cannot be called concurrently.
type Reader interface {
	// Read returns the next log entry, or io.EOF when there are no more log
	// entries. Calling Read on a closed reader will return an error. If a
	// non-nil error is returned, the returned entry is guaranteed to be nil.
	Read(context.Context) (*protos.LogEntry, error)

	// Close closes the Reader. Close can safely be called multiple times.
	Close()
}
