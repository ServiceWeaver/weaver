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
	"io"

	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// remoteLogger collects log entries into batches and sends these batches to a
// specified function.
type remoteLogger struct {
	c        chan *protos.LogEntry
	fallback io.Writer              // Fallback destination when dst() returns an error
	pp       *logging.PrettyPrinter // Used when sending to dst fails
}

const logBufferCount = 1000

func newRemoteLogger(fallback io.Writer) *remoteLogger {
	rl := &remoteLogger{
		c:        make(chan *protos.LogEntry, logBufferCount),
		fallback: fallback,
		pp:       logging.NewPrettyPrinter(false),
	}
	return rl
}

func (rl *remoteLogger) log(entry *protos.LogEntry) {
	// TODO(sanjay): Drop if too many entries are buffered?
	rl.c <- entry
}

// run collects log entries passed to log() and, and passes theme to dst. At
// most one call to dst is outstanding at a time. Log entries that arrive while
// a call is in progress are buffered and sent in the next call.
func (rl *remoteLogger) run(ctx context.Context, dst func(context.Context, *protos.LogEntryBatch) error) {
	batch := &protos.LogEntryBatch{}
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-rl.c:
			// Batch together all available entries.
			batch.Entries = append(batch.Entries, e)
		readloop:
			for {
				select {
				case <-ctx.Done():
					return
				case e := <-rl.c:
					batch.Entries = append(batch.Entries, e)
				default:
					break readloop
				}
			}

			// Send this batch
			if err := dst(ctx, batch); err != nil {
				// Fallback by writing to rl.fallback.
				attr := err.Error()
				for _, e := range batch.Entries {
					e.Attrs = append(e.Attrs, "serviceweaver/logerror", attr)
					fmt.Fprintln(rl.fallback, rl.pp.Format(e))
				}
			}
			batch.Entries = batch.Entries[:0]
		}
	}
}
