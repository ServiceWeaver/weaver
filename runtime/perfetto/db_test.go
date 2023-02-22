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
func storeSpans(ctx context.Context, t *testing.T, db *DB, app, version string, spans ...sdktrace.ReadOnlySpan) {
	if err := db.Store(ctx, app, version, spans); err != nil {
		t.Fatal(err)
	}
}

// makeSpan creates a test span with the given information.
func makeSpan(name string, dur time.Duration, pid int, cgroup string) sdktrace.ReadOnlySpan {
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
				Key:   "coloc_group",
				Value: attribute.StringValue(cgroup),
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
	db, err := open(ctx, fname)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Store a bunch of spans.
	s1 := makeSpan("s1", time.Minute, 1, "cg1")
	s2 := makeSpan("s2", time.Second, 2, "cg1")
	s3 := makeSpan("s3", 2*time.Minute, 3, "cg2")
	s4 := makeSpan("s4", time.Minute, 5, "cg3")
	s5 := makeSpan("s5", time.Minute, 6, "cg4")
	s6 := makeSpan("s6", time.Minute, 7, "cg5")
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
	// application versions and their colocation groups. Ensure that different
	// replicas of the same colocation group get assigned sequential replica
	// numbers, starting with zero.
	ctx := context.Background()
	fname := filepath.Join(t.TempDir(), "tracedb.db_test.db")
	db, err := open(ctx, fname)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expect := func(app, version, cgroup, replicaID string, num int) {
		rn, err := db.getReplicaNumber(ctx, app, version, cgroup, replicaID)
		if err != nil {
			t.Fatal(err)
		}
		if rn != num {
			t.Errorf("unexpected replica for %s %s %s %s; want %d, got %d", app, version, cgroup, replicaID, num, rn)
		}
	}
	expect("app1", "v1", "cg1", "rid1", 0)
	expect("app1", "v1", "cg1", "rid1", 0)
	expect("app1", "v1", "cg1", "rid2", 1)
	expect("app1", "v1", "cg1", "rid3", 2)
	expect("app1", "v1", "cg2", "rid1", 0)
	expect("app1", "v1", "cg2", "rid1", 0)
	expect("app1", "v1", "cg2", "rid2", 1)
	expect("app1", "v2", "cg1", "rid1", 0)
	expect("app1", "v2", "cg1", "rid1", 0)
	expect("app1", "v2", "cg1", "rid2", 1)
	expect("app2", "v1", "cg1", "rid1", 0)
	expect("app2", "v1", "cg1", "rid1", 0)
	expect("app2", "v1", "cg1", "rid2", 1)
}
