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
package perfetto

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	lru "github.com/hashicorp/golang-lru/v2"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
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

type replicaCacheKey struct {
	app, version, weaveletId string
}

// DB is a trace database that stores traces on the local file system.
//
// Trace data is stored in a JSON format [1] that is compatible with the
// Perfetto UI[2].
//
// For each trace span, the following Perfetto events are generated:
//   - A complete event, encapsulating the span duration.
//   - A number of metadata events, encapsulating the span events.
//
// [1] https://docs.google.com/document/d/1CvAClvFfyA5R-PhYUmn5OOQtYMH4h6I0nSsKchNAySU/preview#
// [2] https://ui.perfetto.dev/
type DB struct {
	// Trace data is stored in a sqlite DB spread across three tables:
	// (1) traces:           trace data in a Perfetto-UI-compattible JSON format
	// (2) replica_num:      map from colocation group replica id to a replica
	//                       number
	// (3) next_replica_num: the next replica number to use for a given
	//                       colocation group
	fname string
	db    *sql.DB

	// Cache of replica numbers.
	replicaNumCache *lru.Cache[replicaCacheKey, int]
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

	// Initialize the replica number cache.
	const cacheSize = 65536
	cache, err := lru.New[replicaCacheKey, int](cacheSize)
	if err != nil {
		return nil, err
	}

	t := &DB{
		fname:           fname,
		db:              db,
		replicaNumCache: cache,
	}

	const initTables = `
-- Trace data.
CREATE TABLE IF NOT EXISTS traces (
	app TEXT NOT NULL,
	version TEXT NOT NULL,
	events TEXT NOT NULL
);

-- Map from a weavelet id to a replica number.
CREATE TABLE IF NOT EXISTS replica_num (
	app TEXT NOT NULL,
	version TEXT NOT NULL,
	weavelet_id TEXT NOT NULL,
	num INTEGER NOT NULL,
	PRIMARY KEY(app,version,weavelet_id)
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
func (d *DB) Store(ctx context.Context, app, version string, spans []sdktrace.ReadOnlySpan) error {
	encoded, err := d.encodeSpans(ctx, app, version, spans)
	if err != nil {
		return err
	}
	return d.storeEncoded(ctx, app, version, encoded)
}

func (d *DB) storeEncoded(ctx context.Context, app, version string, encoded []byte) error {
	const stmt = `
		INSERT INTO traces(app, version, events)
		VALUES (?,?,?);
	`
	_, err := d.execDB(ctx, stmt, app, version, string(encoded))
	return err
}

func (d *DB) encodeSpans(ctx context.Context, app, version string, spans []sdktrace.ReadOnlySpan) ([]byte, error) {
	var b bytes.Buffer
	for _, span := range spans {
		if err := d.encodeSpan(ctx, app, version, span, &b); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

func (d *DB) encodeSpan(ctx context.Context, app, version string, span sdktrace.ReadOnlySpan, buf *bytes.Buffer) error {
	appendEvent := func(event any) error {
		b, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if buf.Len() > 0 {
			buf.Write([]byte{','})
		}
		_, err = buf.Write(b)
		return err
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

	// Get replica number that corresponds to the colocation group replica
	// that generated the span. We do this to avoid displaying long random
	// numbers on the Perfetto UI.
	replicaNum, err := d.getReplicaNumber(ctx, app, version, weaveletId)
	if err != nil {
		return err
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
		Tid:  replicaNum,
		Args: map[string]string{"name": fmt.Sprintf("Weavelet %d", replicaNum)},
	}); err != nil {
		return err
	}
	if err := appendEvent(&metadataEvent{
		Ph:   "M", // make it a metadata event
		Name: "thread_name",
		Cat:  span.SpanKind().String(),
		Pid:  weaveletFP,
		Tid:  replicaNum,
		Args: map[string]string{"name": "Weavelet"},
	}); err != nil {
		return err
	}

	// Build a complete event.
	if err := appendEvent(&completeEvent{
		Ph:   "X", // make it a complete event
		Name: eventName,
		Cat:  span.SpanKind().String(),
		Pid:  weaveletFP,
		Tid:  replicaNum,
		Args: args,
		Ts:   span.StartTime().UnixMicro(),
		Dur:  span.EndTime().UnixMicro() - span.StartTime().UnixMicro(),
	}); err != nil {
		return err
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
			Tid:  replicaNum,
			Args: attrs,
		}); err != nil {
			return err
		}
	}
	return nil
}

// getReplicaNumber returns a replica number associated with the given
// weavelet. If no such number exists, a new number will be associated. The
// returned replica number is guaranteed to come from a dense number space.
func (d *DB) getReplicaNumber(ctx context.Context, app, version, weaveletId string) (int, error) {
	// Try to return the replica number from the cache.
	cacheKey := replicaCacheKey{app, version, weaveletId}
	if replicaNum, ok := d.replicaNumCache.Get(cacheKey); ok {
		return replicaNum, nil
	}

	// Get the replica number from the database.
	replicaNum, err := d.getReplicaNumberUncached(ctx, app, version, weaveletId)
	if err == nil {
		d.replicaNumCache.Add(cacheKey, replicaNum)
	}
	return replicaNum, err
}

func (d *DB) getReplicaNumberUncached(ctx context.Context, app, version, weaveletId string) (int, error) {
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelLinearizable})
	if err != nil {
		return -1, err
	}
	defer tx.Rollback() //nolint:errcheck // rollback errors can be ignored

	// See if the replica number is already associated.
	const query = `
		SELECT num
		FROM replica_num
		WHERE app=? AND version=? AND weavelet_id=?;
	`
	rows, err := tx.QueryContext(ctx, query, app, version, weaveletId)
	if err != nil {
		return -1, err
	}
	defer rows.Close()
	if rows.Next() { // already associated
		var replicaNum int
		err = rows.Scan(&replicaNum)
		return replicaNum, err
	}
	if err := rows.Err(); err != nil {
		return -1, err
	}

	// Compute the next replica number.
	const nextReplicaNumberQuery = `
		SELECT COALESCE(MAX(num), -1)
		FROM replica_num
		WHERE app=? AND version=?;
	`
	rows, err = tx.QueryContext(ctx, nextReplicaNumberQuery, app, version)
	if err != nil {
		return -1, err
	}
	defer rows.Close()
	replicaNum := 0
	if rows.Next() {
		if err = rows.Scan(&replicaNum); err != nil {
			return -1, err
		}
		replicaNum++
	}
	if err := rows.Err(); err != nil {
		return -1, err
	}

	// Associate the next replica number.
	const stmt = `
		INSERT INTO replica_num(app, version, weavelet_id, num)
		VALUES (?,?,?,?);
	`
	if _, err = tx.ExecContext(ctx, stmt, app, version, weaveletId, replicaNum); err != nil {
		return -1, err
	}

	// Commit the transaction.
	err = tx.Commit()
	return replicaNum, err
}

// fetch returns all trace events for the given application version.
func (d *DB) fetch(ctx context.Context, app, version string) ([]byte, error) {
	const query = `
		SELECT GROUP_CONCAT(events, ',')
		FROM traces
		WHERE
		(app=? OR ?="") AND (version=? OR ?="");
	`
	rows, err := d.queryDB(ctx, query, app, app, version, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var traces []byte
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		// no record
		return traces, nil
	}
	if err := rows.Scan(&traces); err != nil {
		return nil, err
	}
	return traces, nil
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
//	https://ui.perfetto.dev/#!/?url=http://<hostname>:9001?app=<app>&version=<version>
//
// , where <hostname> is the hostname of the machine running this server
// (e.g., "127.0.0.1"), <app> is the application name, and <version> is the
// application version. If <version> is empty, all of the application's traces
// will be displayed. If <app> is also empty, all of the database traces will be
// displayed.
//
// Perfetto UI requires that the server runs on the local port 9001. For that
// reason, this method will block until port 9001 becomes available.
func (d *DB) Serve(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("OK")) //nolint:errcheck // response write error
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		app := r.URL.Query().Get("app")
		version := r.URL.Query().Get("version")

		// Set access control according to:
		//   https://perfetto.dev/docs/visualization/deep-linking-to-perfetto-ui.
		w.Header().Set("Access-Control-Allow-Origin", "https://ui.perfetto.dev")

		data, err := d.fetch(r.Context(), app, version)
		if err != nil || len(data) == 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		traces := make([]byte, len(data)+2)
		traces[0] = '['
		copy(traces[1:], data)
		traces[len(data)+1] = ']'
		w.Write(traces) //nolint:errcheck // response write error
	})
	server := http.Server{Handler: mux}
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

// isLocked returns whether the error is a "database is locked" error.
func isLocked(err error) bool {
	sqlError := &sqlite.Error{}
	ok := errors.As(err, &sqlError)
	return ok && (sqlError.Code() == sqlite3.SQLITE_BUSY || sqlError.Code() == sqlite3.SQLITE_LOCKED)
}

func fp(name string) int {
	hasher := sha256.New()
	hasher.Write([]byte(name))
	return int(binary.LittleEndian.Uint32(hasher.Sum(nil)))
}
