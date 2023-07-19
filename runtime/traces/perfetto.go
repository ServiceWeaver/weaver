// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package traces

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

// ServePerfetto runs an HTTP server that serves traces stored in the database
// to the Chrome Browser's Perfetto UI.
//
// In order to view the traces, open the following URL in a Chrome browser:
//
//	https://ui.perfetto.dev/#!/?url=http://<hostname>:9001?trace_id=<trace_id>
//
// , where <hostname> is the hostname of the machine running this server
// (e.g., "127.0.0.1"), and <trace_id> is the trace ID in hexadecimal format.
//
// Perfetto UI requires that the server runs on local port 9001. For that
// reason, this method will block until port 9001 becomes available.
func ServePerfetto(ctx context.Context, db *DB) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("OK")) //nolint:errcheck // response write error
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Set access control according to:
		//   https://perfetto.dev/docs/visualization/deep-linking-to-perfetto-ui.
		w.Header().Set("Access-Control-Allow-Origin", "https://ui.perfetto.dev")
		traceID := r.URL.Query().Get("trace_id")
		data, err := fetchSpans(r.Context(), db, traceID)
		if err != nil {
			http.Error(w, "cannot fetch trace", http.StatusInternalServerError)
			return
		}
		w.Write(data) //nolint:errcheck // response write error
	})
	server := http.Server{Handler: mux}

	// Repeatedly try to start a Perfetto HTTP server on port 9001. As long as
	// we fail, it means that some other OS process is serving database
	// traces on that port, and is serving "our" traces as well.
	// TODO(spetrovic): With recent "purge" changes, this is no longer the case,
	// as we now have a separate database for single, multi, and GKE-local
	// deployers. We should fix this, possibly by using these instructions [1].
	//
	// [1]: https://perfetto.dev/docs/visualization/deep-linking-to-perfetto-ui
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

// fetchSpans retrieves and encodes the spans in a format that can be read
// by the Perfetto UI.
func fetchSpans(ctx context.Context, db *DB, traceID string) ([]byte, error) {
	if traceID == "" {
		return nil, fmt.Errorf("invalid trace id %q", traceID)
	}
	spans, err := db.FetchSpans(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch spans: %w", err)
	}
	if len(spans) == 0 {
		return nil, fmt.Errorf("cannot find span")
	}
	data, err := encodeSpans(spans)
	if err != nil {
		return nil, fmt.Errorf("cannot encode spans: %w", err)
	}
	return data, nil
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
		s := &ReadSpan{Span: span}
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

// completeEvent renders a Perfetto event that contains a start time and a
// duration.
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

// metadataEvent renders a Perfetto event that contains metadata information
// for a completeEvent.
type metadataEvent struct {
	Ph   string            `json:"ph"`   // the event type
	Name string            `json:"name"` // name of the event
	Cat  string            `json:"cat"`  // category of the event
	Pid  int               `json:"pid"`  // the id of the process that output the event
	Tid  int               `json:"tid"`  // the thread id of the thread that output the event
	Args map[string]string `json:"args"` // arguments provided for the event
}

func fp(name string) int {
	hasher := sha256.New()
	hasher.Write([]byte(name))
	return int(binary.LittleEndian.Uint32(hasher.Sum(nil)))
}
