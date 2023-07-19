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

package traces_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/traces"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// Current time, rounded to a whole number of microseconds.
var now = time.UnixMicro(time.Now().UnixMicro())

// storeSpans stores the provided application version's spans in the database.
func storeSpans(ctx context.Context, t *testing.T, db *traces.DB, app, version string, spans ...*protos.Span) {
	if err := db.Store(ctx, app, version, &protos.TraceSpans{Span: spans}); err != nil {
		t.Fatal(err)
	}
}

// makeSpan creates a test span with the given information.
func makeSpan(name string, tid, sid, pid string, start, end time.Time, attrs ...string) *protos.Span {
	protoAttrs := make([]*protos.Span_Attribute, len(attrs))
	for i, attr := range attrs {
		parts := strings.Split(attr, "=")
		if len(parts) != 2 {
			panic("attribute format not key=val")
		}
		val := &protos.Span_Attribute_Value{}
		if valInt, err := strconv.Atoi(parts[1]); err == nil {
			val.Type = protos.Span_Attribute_Value_INT64
			val.Value = &protos.Span_Attribute_Value_Num{Num: uint64(valInt)}
		} else {
			val.Type = protos.Span_Attribute_Value_STRING
			val.Value = &protos.Span_Attribute_Value_Str{Str: parts[1]}
		}
		protoAttrs[i] = &protos.Span_Attribute{
			Key:   parts[0],
			Value: val,
		}
	}
	return &protos.Span{
		Name:         name,
		TraceId:      []byte(tid),
		SpanId:       []byte(sid),
		ParentSpanId: []byte(pid),
		Kind:         protos.Span_SERVER,
		StartMicros:  start.UnixMicro(),
		EndMicros:    end.UnixMicro(),
		Attributes:   protoAttrs,
	}
}

func tid(id int) string {
	return fmt.Sprintf("%016d", id)
}

func sid(id int) string {
	if id == 0 {
		return string(make([]byte, 8))
	}
	return fmt.Sprintf("%08d", id)
}

func tick(t int) time.Time {
	if t == 0 {
		return time.Time{}
	}
	return now.Add(time.Duration(t) * time.Second)
}

func TestQueryTraces(t *testing.T) {
	ctx := context.Background()
	fname := filepath.Join(t.TempDir(), "tracedb.db_test.db")
	db, err := traces.OpenDB(ctx, fname)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	dur := func(ts int) time.Duration {
		return time.Duration(ts) * time.Second
	}

	// Store a bunch of spans.
	s1 := makeSpan("s1", tid(1), sid(1), sid(0), tick(3), tick(10))
	s2 := makeSpan("s2", tid(1), sid(2), sid(1), tick(4), tick(9))
	s3 := makeSpan("s3", tid(1), sid(3), sid(1), tick(5), tick(6))
	s4 := makeSpan("s4", tid(2), sid(4), sid(0), tick(1), tick(7), "http.status_code=200")
	s5 := makeSpan("s5", tid(2), sid(5), sid(4), tick(3), tick(7))
	s6 := makeSpan("s6", tid(2), sid(6), sid(5), tick(4), tick(6))
	s7 := makeSpan("s7", tid(3), sid(7), sid(0), tick(2), tick(6), "http.status_code=400")
	s8 := makeSpan("s8", tid(3), sid(8), sid(7), tick(3), tick(5))
	s9 := makeSpan("s9", tid(3), sid(9), sid(7), tick(3), tick(4))
	storeSpans(ctx, t, db, "app1", "v1", s1, s2, s3)
	storeSpans(ctx, t, db, "app1", "v2", s4, s5, s6)
	storeSpans(ctx, t, db, "app2", "v1", s7, s8, s9)

	// Issue queries and verify that the results are as expected.
	for _, tc := range []struct {
		help               string
		app                string
		version            string
		start, end         time.Time
		durLower, durUpper time.Duration
		onlyErrs           bool
		limit              int64
		expect             []traces.TraceSummary
	}{
		{
			help: "all",
			expect: []traces.TraceSummary{
				{tid(1), tick(3), tick(10), "OK"},
				{tid(2), tick(1), tick(7), "OK"},
				{tid(3), tick(2), tick(6), "Bad Request"},
			},
		},
		{
			help: "match app",
			app:  "app1",
			expect: []traces.TraceSummary{
				{tid(1), tick(3), tick(10), "OK"},
				{tid(2), tick(1), tick(7), "OK"},
			},
		},
		{
			help:    "match version",
			version: "v1",
			expect: []traces.TraceSummary{
				{tid(1), tick(3), tick(10), "OK"},
				{tid(3), tick(2), tick(6), "Bad Request"},
			},
		},
		{
			help:    "match app version",
			app:     "app1",
			version: "v1",
			expect: []traces.TraceSummary{
				{tid(1), tick(3), tick(10), "OK"},
			},
		},
		{
			help:  "match start time",
			start: tick(2),
			expect: []traces.TraceSummary{
				{tid(1), tick(3), tick(10), "OK"},
				{tid(3), tick(2), tick(6), "Bad Request"},
			},
		},
		{
			help: "match end time",
			end:  tick(9),
			expect: []traces.TraceSummary{
				{tid(2), tick(1), tick(7), "OK"},
				{tid(3), tick(2), tick(6), "Bad Request"},
			},
		},
		{
			help:     "match duration lower",
			durLower: dur(5),
			expect: []traces.TraceSummary{
				{tid(1), tick(3), tick(10), "OK"},
				{tid(2), tick(1), tick(7), "OK"},
			},
		},
		{
			help:     "match duration upper",
			durUpper: dur(7),
			expect: []traces.TraceSummary{
				{tid(2), tick(1), tick(7), "OK"},
				{tid(3), tick(2), tick(6), "Bad Request"},
			},
		},
		{
			help:     "match only errors",
			onlyErrs: true,
			expect: []traces.TraceSummary{
				{tid(3), tick(2), tick(6), "Bad Request"},
			},
		},
		{
			help:  "match limit",
			limit: 2,
			expect: []traces.TraceSummary{
				{tid(1), tick(3), tick(10), "OK"},
				{tid(2), tick(1), tick(7), "OK"},
			},
		},
	} {
		t.Run(tc.help, func(t *testing.T) {
			actual, err := db.QueryTraces(ctx, tc.app, tc.version, tc.start, tc.end, tc.durLower, tc.durUpper, tc.onlyErrs, tc.limit)
			if err != nil {
				t.Fatal(err)
			}
			// Undo the hex conversion for tests.
			for i, trace := range actual {
				s, err := hex.DecodeString(trace.TraceID)
				if err != nil {
					t.Fatal(err)
				}
				actual[i].TraceID = string(s)
			}
			less := func(a, b traces.TraceSummary) bool {
				return string(a.TraceID[:]) < string(b.TraceID[:])
			}
			if diff := cmp.Diff(tc.expect, actual, cmpopts.SortSlices(less)); diff != "" {
				t.Errorf("unexpected traces: (-want +got): %s", diff)
			}
		})
	}
}

func BenchmarkStore(b *testing.B) {
	ctx := context.Background()
	s := makeSpan("s1", tid(1), sid(1), sid(1), tick(3), tick(10))
	for _, size := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("%d", size), func(b *testing.B) {
			fname := filepath.Join(b.TempDir(), "tracedb.db_bench.db")
			db, err := traces.OpenDB(ctx, fname)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { db.Close() })

			spans := &protos.TraceSpans{}
			spans.Span = make([]*protos.Span, size)
			for i := 0; i < size; i++ {
				spans.Span[i] = s
			}
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				if err := db.Store(ctx, "app", "v", spans); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
