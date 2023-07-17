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
//
// TODO(spetrovic): Move this package to internal/
package perfetto

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// completeEvent renders an event that contains a start time and a duration.
type completeEvent struct {
	Ph   string                       `json:"ph"`   // the event type
	Name string                       `json:"name"` // name of the event
	Cat  string                       `json:"cat"`  // category of the event
	Pid  int                          `json:"pid"`  // the id of the process that output the event
	Tid  int                          `json:"tid"`  // the thread id of the thread that output the event
	Ts   int64                        `json:"ts"`   // start time of the event
	Dur  int64                        `json:"dur"`  // duration of the event
	Args map[string]map[string]string `json:"args"` // arguments provided for the event
}

// metadataEvent renders an event that contains metadata information for a completeEvent.
type metadataEvent struct {
	Ph   string            `json:"ph"`   // the event type
	Name string            `json:"name"` // name of the event
	Cat  string            `json:"cat"`  // category of the event
	Pid  int               `json:"pid"`  // the id of the process that output the event
	Tid  int               `json:"tid"`  // the thread id of the thread that output the event
	Args map[string]string `json:"args"` // arguments provided for the event
}

// DB is a trace database that stores traces on the local file system.
//
// Trace spans are stored in a single sqlite DB table. Each span is stored in
// a separate row in the table.
//
// TODO(spetrovic): Separate the database from the Pefetto serving code.
type DB struct {
	// Trace data is stored in a sqlite DB spread across two tables:
	// (1) spans:           serialized trace spans
	// (2) span_attributes: span attributes stored in key,value format
	fname string
	db    *sql.DB
}

// Open opens the default trace database on the local machine, which is
// persisted in the provided file.
func Open(ctx context.Context, fname string) (*DB, error) {
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

	const initTables = `
-- Queryable trace data.
CREATE TABLE IF NOT EXISTS traces (
	trace_id TEXT NOT NULL,
	app TEXT NOT NULL,
	version TEXT NOT NULL,
	name TEXT,
	start_time_unix_us INTEGER,
	end_time_unix_us INTEGER,
	PRIMARY KEY(trace_id)
);

-- Queryable trace attributes.
CREATE TABLE IF NOT EXISTS trace_attributes (
	trace_id TEXT NOT NULL,
	key TEXT NOT NULL,
	val TEXT,
	FOREIGN KEY (trace_id) REFERENCES traces (trace_id)
);

-- Encoded spans.
CREATE TABLE IF NOT EXISTS encoded_spans (
	trace_id TEXT NOT NULL,
	data TEXT,
	FOREIGN KEY (trace_id) REFERENCES traces (trace_id)
);
`
	if _, err := t.execDB(ctx, initTables); err != nil {
		return nil, fmt.Errorf("open trace DB %s: %w", fname, err)
	}

	return t, nil
}

// Close closes the trace database.
func (d *DB) Close() error {
	return d.db.Close()
}

// Store stores the given traces in the database.
func (d *DB) Store(ctx context.Context, app, version string, spans *protos.TraceSpans) error {
	// NOTE: we insert all rows transactionally, as it is significantly faster
	// than inserting one row at a time [1].
	//
	// [1]: https://stackoverflow.com/questions/1711631/improve-insert-per-second-performance-of-sqlite
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelLinearizable})
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // supplanted by errs below
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
	const traceStmt = `
INSERT INTO traces(trace_id,app,version,name,start_time_unix_us,end_time_unix_us)
VALUES (?,?,?,?,?,?)`
	_, err := tx.ExecContext(ctx, traceStmt, hex.EncodeToString(root.TraceId), app, version, root.Name, root.StartMicros, root.EndMicros)
	if err != nil {
		return fmt.Errorf("write span to %s: %w", d.fname, err)
	}

	const attrStmt = `INSERT INTO trace_attributes VALUES(?,?,?)`
	for _, attr := range root.Attributes {
		valStr := attributeValueString(attr)
		_, err := tx.ExecContext(ctx, attrStmt, hex.EncodeToString(root.TraceId), attr.Key, valStr)
		if err != nil {
			return fmt.Errorf("write trace attributes to %s: %w", d.fname, err)
		}
	}
	return nil
}

func (d *DB) storeSpan(ctx context.Context, tx *sql.Tx, span *protos.Span) error {
	encoded, err := proto.Marshal(span)
	if err != nil {
		return err
	}
	const stmt = `INSERT INTO encoded_spans(trace_id,data) VALUES (?,?)`
	_, err = tx.ExecContext(ctx, stmt, hex.EncodeToString(span.TraceId), encoded)
	return err
}

// TraceSummary stores summary information about a trace.
type TraceSummary struct {
	TraceID            string    // Unique trace identifier, in hex format.
	StartTime, EndTime time.Time // Start and end times for the trace.

	// TODO(spetrovic): Add more fields as we figure out what type of
	// summary trace information we want to display.
}

// QueryTraces returns the summaries of the traces that match the given
// query arguments, namely:
//   - That have been generated by the given application version.
//   - That fit entirely in the given [startTime, endTime] time interval.
//   - Whose duration is in the given [durationLower, durationUpper] range.
//   - Who are in the most recent limit of trace spans.
//
// Any query argument that has a zero value (e.g., empty app or version,
// zero endTime) is ignored, i.e., it matches all spans.
//
// TODO(spetrovic): Add attribute matching.
func (d *DB) QueryTraces(ctx context.Context, app, version string, startTime, endTime time.Time, durationLower, durationUpper time.Duration, limit int64) ([]TraceSummary, error) {
	const query = `
SELECT trace_id, start_time_unix_us, end_time_unix_us
FROM traces
WHERE
	(app=? OR ?="") AND (version=? OR ?="") AND
	(start_time_unix_us>=? OR ?=0) AND (end_time_unix_us<=? OR ?=0) AND
	((end_time_unix_us - start_time_unix_us)>=? OR ?=0) AND
	((end_time_unix_us - start_time_unix_us)<=? OR ?=0)
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
	rows, err := d.queryDB(ctx, query, app, app, version, version, startTimeUs, startTimeUs, endTimeUs, endTimeUs, durationLowerUs, durationLowerUs, durationUpperUs, durationUpperUs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var traces []TraceSummary
	for rows.Next() {
		var traceID string
		var startTimeUs, endTimeUs int64
		if err := rows.Scan(&traceID, &startTimeUs, &endTimeUs); err != nil {
			return nil, err
		}
		traces = append(traces, TraceSummary{
			TraceID:   traceID,
			StartTime: time.UnixMicro(startTimeUs),
			EndTime:   time.UnixMicro(endTimeUs),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return traces, nil
}

func (d *DB) fetchSpans(ctx context.Context, traceID string) ([]*protos.Span, error) {
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

// Serve runs an HTTP server that serves traces stored in the database.
// In order to view the traces, open the following URL in a Chrome browser:
//
//	https://ui.perfetto.dev/#!/?url=http://<hostname>:9001?trace_id=<trace_id>
//
// , where <hostname> is the hostname of the machine running this server
// (e.g., "127.0.0.1"), and <trace_id> is the trace ID in hexadecimal format.
//
// Perfetto UI requires that the server runs on the local port 9001. For that
// reason, this method will block until port 9001 becomes available.
func (d *DB) Serve(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("OK")) //nolint:errcheck // response write error
	})
	mux.HandleFunc("/", d.fetchTrace)
	server := http.Server{Handler: mux}

	// Repeatedly try to start a Perfetto HTTP server on port 9001. As long as
	// we fail, it means that some other OS process is serving database
	// traces on that port, and is serving "our" traces as well.
	// TODO(spetrovic): With recent "purge" changes, this is no longer the case,
	// as we now have a separate database for single, multi, and GKE-local
	// deployers. We should fix this.
	ticker := time.NewTicker(time.Second)
	var warnOnce sync.Once
	for {
		select {
		case <-ticker.C:
			lis, err := net.Listen("tcp", "localhost:9001")
			if err != nil {
				warnOnce.Do(func() {
					fmt.Fprintf(os.Stderr, "Perfetto server failed to listen on port 9001: %v\n", err)
					fmt.Fprintf(os.Stderr, "Perfetto server will retry until port 9001 is available\n")
				})
				continue
			}

			ticker.Stop()
			err = server.Serve(lis)
			if !errors.Is(err, http.ErrServerClosed) {
				fmt.Fprintf(os.Stderr, "Failed to serve perfetto backend: %v\n", err)
			}
			return

		case <-ctx.Done():
			return
		}
	}
}

func (d *DB) fetchTrace(w http.ResponseWriter, r *http.Request) {
	// Set access control according to:
	//   https://perfetto.dev/docs/visualization/deep-linking-to-perfetto-ui.
	w.Header().Set("Access-Control-Allow-Origin", "https://ui.perfetto.dev")
	traceID := r.URL.Query().Get("trace_id")
	if traceID == "" {
		http.Error(w, "no trace id provided", http.StatusBadRequest)
		return
	}
	spans, err := d.fetchSpans(r.Context(), traceID)
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot fetch spans: %v", err), http.StatusInternalServerError)
		return
	}
	if len(spans) == 0 {
		http.Error(w, "cannot find span", http.StatusNotFound)
		return
	}
	traces, err := encodeSpans(spans)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Write(traces) //nolint:errcheck // response write error
}

// encodeSpans encodes the given spans in a format that can be read by the
// Perfetto UI.
func encodeSpans(spans []*protos.Span) ([]byte, error) {
	var buf strings.Builder

	// We are returning a JSON array, so surround everything in [].
	buf.WriteByte('[')
	addEvent := func(event []byte) {
		if buf.Len() > 1 { // NOTE: buf always starts with a '['
			buf.WriteByte(',')
		}
		buf.Write(event)
	}
	for _, span := range spans {
		s := &traceio.ReadSpan{Span: span}
		events, err := encodeSpanEvents(s)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			addEvent(event)
		}
	}
	buf.WriteByte(']')
	return []byte(buf.String()), nil
}

// encodeSpanEvents encodes the given span into a series of Perfetto events.
func encodeSpanEvents(span sdktrace.ReadOnlySpan) ([][]byte, error) {
	var ret [][]byte
	appendEvent := func(event any) error {
		b, err := json.Marshal(event)
		if err != nil {
			return err
		}
		ret = append(ret, b)
		return nil
	}

	// Extract information from the span attributes.
	var pid int
	var weaveletId string
	for _, a := range span.Resource().Attributes() {
		switch a.Key {
		case semconv.ProcessPIDKey:
			pid = int(a.Value.AsInt64())
		case traceio.WeaveletIdTraceKey:
			weaveletId = a.Value.AsString()
		}
	}

	// The span name contains the name of the method that is called. Based
	// on the span kind, we attach an additional label to identify whether
	// the event happened at the client, server, or locally.
	eventName := span.Name()
	switch span.SpanKind() {
	case trace.SpanKindServer:
		eventName = eventName + " [server]"
	case trace.SpanKindClient:
		eventName = eventName + " [client]"
	case trace.SpanKindInternal:
		eventName = eventName + " [local]"
	}

	// Build the attributes map.
	attrs := map[string]string{}
	for _, a := range span.Attributes() {
		attrs[string(a.Key)] = a.Value.Emit()
	}

	// Build the arguments.
	args := map[string]map[string]string{
		"ids": {
			"spanID":        span.SpanContext().SpanID().String(),
			"traceID":       span.SpanContext().TraceID().String(),
			"parentSpanID":  span.Parent().SpanID().String(),
			"parentTraceID": span.Parent().TraceID().String(),
			"processID":     strconv.Itoa(pid),
		},
		"attributes": attrs,
	}

	// Generate a complete event and a series of metadata events.
	weaveletFP := fp(weaveletId)

	// Build two metadata events for each colocation group (one to replace the
	// process name label and one for the thread name).
	if err := appendEvent(&metadataEvent{
		Ph:   "M", // make it a metadata event
		Name: "process_name",
		Cat:  span.SpanKind().String(),
		Pid:  weaveletFP,
		Tid:  weaveletFP,
		Args: map[string]string{"name": "Weavelet"},
	}); err != nil {
		return nil, err
	}
	if err := appendEvent(&metadataEvent{
		Ph:   "M", // make it a metadata event
		Name: "thread_name",
		Cat:  span.SpanKind().String(),
		Pid:  weaveletFP,
		Tid:  weaveletFP,
		Args: map[string]string{"name": "Weavelet"},
	}); err != nil {
		return nil, err
	}

	// Build a complete event.
	if err := appendEvent(&completeEvent{
		Ph:   "X", // make it a complete event
		Name: eventName,
		Cat:  span.SpanKind().String(),
		Pid:  weaveletFP,
		Tid:  weaveletFP,
		Args: args,
		Ts:   span.StartTime().UnixMicro(),
		Dur:  span.EndTime().UnixMicro() - span.StartTime().UnixMicro(),
	}); err != nil {
		return nil, err
	}

	// For each span event, create a corresponding metadata event.
	for _, e := range span.Events() {
		attrs := map[string]string{}
		for _, a := range e.Attributes {
			attrs[string(a.Key)] = a.Value.Emit()
		}
		if err := appendEvent(&metadataEvent{
			Ph:   "M", // make it a metadata event
			Name: e.Name,
			Cat:  span.SpanKind().String(),
			Pid:  weaveletFP,
			Tid:  weaveletFP,
			Args: attrs,
		}); err != nil {
			return nil, err
		}
	}
	return ret, nil
}

func attributeValueString(attr *protos.Span_Attribute) string {
	switch v := attr.Value.Value.(type) {
	case *protos.Span_Attribute_Value_Num:
		return fmt.Sprintf("%d", v.Num)
	case *protos.Span_Attribute_Value_Str:
		return v.Str
	case *protos.Span_Attribute_Value_Nums:
		return fmt.Sprintf("%v", v.Nums)
	case *protos.Span_Attribute_Value_Strs:
		return fmt.Sprintf("%v", v.Strs)
	}
	return ""
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

func fp(name string) int {
	hasher := sha256.New()
	hasher.Write([]byte(name))
	return int(binary.LittleEndian.Uint32(hasher.Sum(nil)))
}
