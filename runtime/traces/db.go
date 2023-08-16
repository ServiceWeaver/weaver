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

// Package perfetto contains libraries for displaying trace information in the
// Perfetto UI.
package traces

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"google.golang.org/protobuf/proto"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// DB is a trace database that stores traces on the local file system.
type DB struct {
	// Trace data is stored in a sqlite DB spread across two tables:
	// (1) traces:         serialized trace data, used for querying.
	// (2) encoded_spans:  full encoded span data, used for fetching all of the
	//                     spans that belong to a given trace.
	fname string
	db    *sql.DB
}

// OpenDB opens the trace database persisted in the provided file. If the
// file doesn't exist, this call creates it.
func OpenDB(ctx context.Context, fname string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(fname), 0700); err != nil {
		return nil, err
	}

	// The DB may be opened by multiple writers. Turn on appropriate
	// concurrency control options. See:
	//   https://www.sqlite.org/pragma.html#pragma_locking_mode
	//   https://www.sqlite.org/pragma.html#pragma_busy_timeout
	const params = "?_locking_mode=NORMAL&_busy_timeout=10000"
	db, err := sql.Open("sqlite", fname+params)
	if err != nil {
		return nil, fmt.Errorf("open perfetto db %q: %w", fname, err)
	}
	db.SetMaxOpenConns(1)

	t := &DB{
		fname: fname,
		db:    db,
	}

	const initDB = `
-- Disable foreign key checking as spans may get inserted into encoded_spans
-- before the corresponding traces are inserted into traces.
PRAGMA foreign_keys=OFF;

-- Queryable trace data.
CREATE TABLE IF NOT EXISTS traces (
	trace_id TEXT NOT NULL,
	app TEXT NOT NULL,
	version TEXT NOT NULL,
	name TEXT,
	start_time_unix_us INTEGER,
	end_time_unix_us INTEGER,
	status TEXT,
	PRIMARY KEY(trace_id)
);

-- Encoded spans.
CREATE TABLE IF NOT EXISTS encoded_spans (
	trace_id TEXT NOT NULL,
	start_time_unix_us INTEGER,
	data TEXT,
	FOREIGN KEY (trace_id) REFERENCES traces (trace_id)
);

-- Garbage-collect traces older than 30 days.
CREATE TRIGGER IF NOT EXISTS expire_traces AFTER INSERT ON traces
BEGIN
	DELETE FROM traces
	WHERE start_time_unix_us < (1000000 * unixepoch('now', '-30 days'));
END;

-- Garbage-collect spans older than 30 days.
CREATE TRIGGER IF NOT EXISTS expire_spans AFTER INSERT ON encoded_spans
BEGIN
	DELETE FROM encoded_spans
	WHERE start_time_unix_us < (1000000 * unixepoch('now', '-30 days'));
END;
`
	if _, err := t.execDB(ctx, initDB); err != nil {
		return nil, fmt.Errorf("open trace DB %s: %w", fname, err)
	}

	return t, nil
}

// Close closes the trace database.
func (d *DB) Close() error {
	return d.db.Close()
}

// Store stores the given trace spans in the database.
func (d *DB) Store(ctx context.Context, app, version string, spans *protos.TraceSpans) error {
	// NOTE: we insert all rows transactionally, as it is significantly faster
	// than inserting one row at a time [1].
	//
	// [1]: https://stackoverflow.com/questions/1711631/improve-insert-per-second-performance-of-sqlite
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelLinearizable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var errs []error
	for _, span := range spans.Span {
		if isRootSpan(span) {
			if err := d.storeTrace(ctx, tx, app, version, span); err != nil {
				errs = append(errs, err)
			}
		}
		if err := d.storeSpan(ctx, tx, span); err != nil {
			errs = append(errs, err)
		}
	}
	if errs != nil {
		return errors.Join(errs...)
	}
	return tx.Commit()
}

func (d *DB) storeTrace(ctx context.Context, tx *sql.Tx, app, version string, root *protos.Span) error {
	const traceStmt = `INSERT INTO traces VALUES (?,?,?,?,?,?,?)`
	_, err := tx.ExecContext(ctx, traceStmt, hex.EncodeToString(root.TraceId), app, version, root.Name, root.StartMicros, root.EndMicros, spanStatus(root))
	return err
}

func (d *DB) storeSpan(ctx context.Context, tx *sql.Tx, span *protos.Span) error {
	encoded, err := proto.Marshal(span)
	if err != nil {
		return err
	}
	const stmt = `INSERT INTO encoded_spans VALUES (?,?,?)`
	_, err = tx.ExecContext(ctx, stmt, hex.EncodeToString(span.TraceId), span.StartMicros, encoded)
	return err
}

// TraceSummary stores summary information about a trace.
type TraceSummary struct {
	TraceID            string    // Unique trace identifier, in hex format.
	StartTime, EndTime time.Time // Start and end times for the trace.
	Status             string    // Trace status string.
}

// QueryTraces returns the summaries of the traces that match the given
// query arguments, namely:
//   - That have been generated by the given application version.
//   - That fit entirely in the given [startTime, endTime] time interval.
//   - Whose duration is in the given [durationLower, durationUpper) range.
//   - Who have an error status.
//   - Who are in the most recent limit of trace spans.
//
// Any query argument that has a zero value (e.g., empty app or version,
// zero endTime) is ignored, i.e., it matches all spans.
func (d *DB) QueryTraces(ctx context.Context, app, version string, startTime, endTime time.Time, durationLower, durationUpper time.Duration, onlyErrors bool, limit int64) ([]TraceSummary, error) {
	const query = `
SELECT trace_id, start_time_unix_us, end_time_unix_us, status
FROM traces
WHERE
	(app=? OR ?="") AND (version=? OR ?="") AND
	(start_time_unix_us>=? OR ?=0) AND (end_time_unix_us<=? OR ?=0) AND
	((end_time_unix_us - start_time_unix_us)>=? OR ?=0) AND
	((end_time_unix_us - start_time_unix_us)<? OR ?=0) AND
	(status != "" OR ?=0)
ORDER BY end_time_unix_us DESC
LIMIT ?
`
	var startTimeUs int64
	if !startTime.IsZero() {
		startTimeUs = startTime.UnixMicro()
	}
	var endTimeUs int64
	if !endTime.IsZero() {
		endTimeUs = endTime.UnixMicro()
	}
	durationLowerUs := durationLower.Microseconds()
	durationUpperUs := durationUpper.Microseconds()
	if limit <= 0 {
		limit = math.MaxInt64
	}
	rows, err := d.queryDB(ctx, query, app, app, version, version, startTimeUs, startTimeUs, endTimeUs, endTimeUs, durationLowerUs, durationLowerUs, durationUpperUs, durationUpperUs, onlyErrors, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var traces []TraceSummary
	for rows.Next() {
		var traceID string
		var startTimeUs, endTimeUs int64
		var status string
		if err := rows.Scan(&traceID, &startTimeUs, &endTimeUs, &status); err != nil {
			return nil, err
		}
		if status == "" {
			status = "OK"
		}
		traces = append(traces, TraceSummary{
			TraceID:   traceID,
			StartTime: time.UnixMicro(startTimeUs),
			EndTime:   time.UnixMicro(endTimeUs),
			Status:    status,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return traces, nil
}

// FetchSpans returns all of the spans that have a given trace id.
func (d *DB) FetchSpans(ctx context.Context, traceID string) ([]*protos.Span, error) {
	const query = `SELECT data FROM encoded_spans WHERE trace_id=?`
	rows, err := d.queryDB(ctx, query, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var spans []*protos.Span
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		span := &protos.Span{}
		if err := proto.Unmarshal(encoded, span); err != nil {
			return nil, err
		}
		spans = append(spans, span)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return spans, nil
}

func (d *DB) queryDB(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	// Keep retrying as long as we are getting the "locked" error.
	for r := retry.Begin(); r.Continue(ctx); {
		rows, err := d.db.QueryContext(ctx, query, args...)
		if isLocked(err) {
			continue
		}
		return rows, err
	}
	return nil, ctx.Err()
}

func (d *DB) execDB(ctx context.Context, query string, args ...any) (sql.Result, error) {
	// Keep retrying as long as we are getting the "locked" error.
	for r := retry.Begin(); r.Continue(ctx); {
		res, err := d.db.ExecContext(ctx, query, args...)
		if isLocked(err) {
			continue
		}
		return res, err
	}
	return nil, ctx.Err()
}

// isLocked returns whether the error is a "database is locked" error.
func isLocked(err error) bool {
	sqlError := &sqlite.Error{}
	ok := errors.As(err, &sqlError)
	return ok && (sqlError.Code() == sqlite3.SQLITE_BUSY || sqlError.Code() == sqlite3.SQLITE_LOCKED)
}

// isRootSpan returns true iff the given span is a root span.
func isRootSpan(span *protos.Span) bool {
	var nilSpanID [8]byte
	return bytes.Equal(span.ParentSpanId, nilSpanID[:])
}

// spanStatus returns the span status string. It returns "" if the status is OK.
func spanStatus(span *protos.Span) string {
	// Look for an error in the span status.
	if span.Status != nil && span.Status.Code == protos.Span_Status_ERROR {
		if span.Status.Error != "" {
			return span.Status.Error
		} else {
			return "unknown error"
		}
	}

	// Look for an HTTP error in the span attributes.
	for _, attr := range span.Attributes {
		if attr.Key != "http.status_code" {
			continue
		}
		if attr.Value.Type != protos.Span_Attribute_Value_INT64 {
			continue
		}
		val, ok := attr.Value.Value.(*protos.Span_Attribute_Value_Num)
		if !ok {
			continue
		}
		if val.Num >= 400 && val.Num < 600 {
			return http.StatusText(int(val.Num))
		}
	}

	// No errors found.
	return ""
}
