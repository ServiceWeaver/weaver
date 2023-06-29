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

package perfetto

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var now = time.Now()

// storeSpans stores the provided application version's spans in the database.
func storeSpans(ctx context.Context, t testing.TB, db *DB, app, version string, spans ...sdktrace.ReadOnlySpan) {
	if err := db.Store(ctx, app, version, spans); err != nil {
		t.Fatal(err)
	}
}

// makeSpan creates a test span with the given information.
func makeSpan(name string, dur time.Duration, pid int, weavelet string) sdktrace.ReadOnlySpan {
	return tracetest.SpanStub{
		Name:      name,
		StartTime: now,
		EndTime:   now.Add(dur),
		SpanKind:  trace.SpanKindServer,
		Attributes: []attribute.KeyValue{
			{
				Key:   semconv.ProcessPIDKey,
				Value: attribute.IntValue(pid),
			},
			{
				Key:   "weavelet",
				Value: attribute.StringValue(weavelet),
			},
		},
	}.Snapshot()
}

func TestStoreFetch(t *testing.T) {
	// Test Plan: insert a number of spans into the database and associate
	// them with various application versions. Then fetch the spans for
	// those application versions and validate they are as expected.
	ctx := context.Background()
	fname := filepath.Join(t.TempDir(), "tracedb.db_test.db")
	db, err := Open(ctx, fname, DBOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Store a bunch of spans.
	s1 := makeSpan("s1", time.Minute, 1, "w1")
	s2 := makeSpan("s2", time.Second, 2, "w1")
	s3 := makeSpan("s3", 2*time.Minute, 3, "w2")
	s4 := makeSpan("s4", time.Minute, 5, "w3")
	s5 := makeSpan("s5", time.Minute, 6, "w4")
	s6 := makeSpan("s6", time.Minute, 7, "w5")
	storeSpans(ctx, t, db, "app1", "v1", s1, s2)
	storeSpans(ctx, t, db, "app1", "v1", s3)
	storeSpans(ctx, t, db, "app1", "v2", s4)
	storeSpans(ctx, t, db, "app2", "v1", s5, s6)

	// Fetch a bunch of spans.
	for _, tc := range []struct {
		help    string
		app     string
		version string
		expect  []sdktrace.ReadOnlySpan
	}{
		{
			help:    "single span",
			app:     "app1",
			version: "v2",
			expect:  []sdktrace.ReadOnlySpan{s4},
		},
		{
			help:    "two spans",
			app:     "app2",
			version: "v1",
			expect:  []sdktrace.ReadOnlySpan{s5, s6},
		},
		{
			help:    "two rows",
			app:     "app1",
			version: "v1",
			expect:  []sdktrace.ReadOnlySpan{s1, s2, s3},
		},
	} {
		t.Run(tc.help, func(t *testing.T) {
			expectEnc, err := db.encodeSpans(ctx, tc.app, tc.version, tc.expect)
			if err != nil {
				t.Fatal(err)
			}
			actualEnc, err := db.fetch(ctx, tc.app, tc.version)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(expectEnc, actualEnc); diff != "" {
				t.Fatalf("unexpected metrics (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReplicaNum(t *testing.T) {
	// Test Plan: Retrieve replica numbers for a number of different
	// application versions. Ensure that different replicas of the same version
	// get assigned sequential replica numbers, starting with zero.
	ctx := context.Background()
	fname := filepath.Join(t.TempDir(), "tracedb.db_test.db")
	db, err := Open(ctx, fname, DBOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expect := func(app, version, weavelet string, num int) {
		rn, err := db.getReplicaNumber(ctx, app, version, weavelet)
		if err != nil {
			t.Fatal(err)
		}
		if rn != num {
			t.Errorf("unexpected replica for %s %s %s; want %d, got %d", app, version, weavelet, num, rn)
		}
	}
	expect("app1", "v1", "w1", 0)
	expect("app1", "v1", "w2", 1)
	expect("app1", "v1", "w3", 2)
	expect("app1", "v2", "w1", 0)
	expect("app1", "v2", "w2", 1)
	expect("app1", "v2", "w3", 2)
	expect("app2", "v1", "w1", 0)
	expect("app2", "v1", "w2", 1)
	expect("app2", "v1", "w3", 2)
	expect("app2", "v2", "w1", 0)
	expect("app2", "v2", "w2", 1)
	expect("app2", "v2", "w3", 2)
}

func TestMaxTraceBytes(t *testing.T) {
	ctx := context.Background()
	const encSize = 1000
	const sizeLimit = 100 * encSize
	encData := []byte(strings.Repeat("x", encSize))

	fname := filepath.Join(t.TempDir(), "tracedb.db_test.db")
	db, err := Open(ctx, fname, DBOptions{MaxTraceBytes: sizeLimit})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	numRows := func() int64 {
		const query = `SELECT COUNT(*) FROM traces`
		rows, err := db.queryDB(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if !rows.Next() {
			t.Fatal("no rows for query")
		}
		var count int64
		if err := rows.Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	// Fill in the table up to the size limit.
	for db.size() < sizeLimit {
		if err := db.storeEncoded(ctx, "app", "v0", encData); err != nil {
			t.Fatal(err)
		}
	}

	// Expect a constant number of rows from this point onward.
	expected := numRows()
	for i := 0; i < 100; i++ {
		if err := db.storeEncoded(ctx, "app", "v0", encData); err != nil {
			t.Fatal(err)
		}
		actual := numRows()
		if actual != expected {
			t.Fatalf("too many rows in table at insertion #%d, want %d, got %d", i, expected, actual)
		}
	}
}

func BenchmarkStore(b *testing.B) {
	ctx := context.Background()
	const encSize = 1000
	encData := []byte(strings.Repeat("x", encSize))
	for _, bm := range []struct {
		name    string
		limit   bool
		maxSize int64
	}{
		{
			name:    "nolimit_small",
			maxSize: 10 * encSize,
		},
		{
			name:    "limit_small",
			limit:   true,
			maxSize: 10 * encSize,
		},
		{
			name:    "nolimit_medium",
			maxSize: 100 * encSize,
		},
		{
			name:    "limit_medium",
			limit:   true,
			maxSize: 100 * encSize,
		},
		{
			name:    "nolimit_big",
			maxSize: 500 * encSize,
		},
		{
			name:    "limit_big",
			limit:   true,
			maxSize: 500 * encSize,
		},
	} {
		b.Run(bm.name, func(b *testing.B) {
			fname := filepath.Join(b.TempDir(), "tracedb.db_bench.db")
			var opts DBOptions
			if bm.limit {
				opts.MaxTraceBytes = bm.maxSize
			}
			db, err := Open(ctx, fname, opts)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { db.Close() })

			store := func() {
				if err := db.storeEncoded(ctx, "app", "v0", encData); err != nil {
					b.Fatal(err)
				}
			}

			// Fill up the DB with traces until it's reach the maxSize
			// (in the limit case).
			for db.size() < int64(bm.maxSize) {
				store()
			}

			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				store()
			}
		})
	}
}
