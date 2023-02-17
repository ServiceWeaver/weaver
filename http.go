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

package weaver

import (
	"net/http"
	"time"

	"github.com/ServiceWeaver/weaver/metrics"
)

// TODO(mwhittaker): Measure the size of HTTP requests.
// TODO(mwhittaker): Measure the size of HTTP replies.
// TODO(mwhittaker): Allow users to disable certain metrics?

type httpLabels struct {
	Label string // user-provided instrumentation label
	Host  string // URL host
}

type httpErrorLabels struct {
	Label string // user-provided instrumentation label
	Host  string // URL host
	Code  int    // HTTP status code (e.g., 404)
}

var (
	httpRequestCounts = metrics.NewCounterMap[httpLabels](
		"serviceweaver_http_request_count",
		"Count of HTTP requests received",
	)
	httpRequestErrors = metrics.NewCounterMap[httpErrorLabels](
		"serviceweaver_http_error_count",
		"Count of HTTP replies with a 4XX or 5XX status code",
	)
	httpRequestLatencyMicros = metrics.NewHistogramMap[httpLabels](
		"serviceweaver_http_request_latency_micros",
		"Duration, in microseconds, of HTTP request execution",
		metrics.NonNegativeBuckets,
	)
	httpRequestBytesReceived = metrics.NewHistogramMap[httpLabels](
		"serviceweaver_http_request_bytes_received",
		"Number of bytes received by HTTP request handlers",
		metrics.NonNegativeBuckets,
	)
	httpRequestBytesReturned = metrics.NewHistogramMap[httpLabels](
		"serviceweaver_http_request_bytes_returned",
		"Number of bytes returned by HTTP request handlers",
		metrics.NonNegativeBuckets,
	)
)

// InstrumentHandler instruments the provided HTTP handler to maintain the
// following metrics about HTTP request execution. Every metric is labelled
// with the supplied label.
//
//   - serviceweaver_http_request_count: Total number of requests.
//   - serviceweaver_http_error_count: Total number of 4XX and 5XX replies.
//   - serviceweaver_http_request_latency_micros: Execution latency in microseconds.
func InstrumentHandler(label string, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// TODO(spetrovic): It is possible for the user to override r.Host
		// and therefore get an incorrect host label attached here. Consider
		// a more robust solution for fetching the hostname (e.g., get the
		// listener attached to the HTTP server and return its associated
		// hostname).
		labels := httpLabels{Label: label, Host: r.Host}

		httpRequestCounts.Get(labels).Add(1)
		defer func() {
			httpRequestLatencyMicros.Get(labels).Put(
				float64(time.Since(start).Microseconds()))
		}()
		if size, ok := requestSize(r); ok {
			httpRequestBytesReceived.Get(labels).Put(float64(size))
		}
		writer := responseWriterInstrumenter{w: w}
		handler.ServeHTTP(&writer, r)
		if writer.statusCode >= 400 && writer.statusCode < 600 {
			httpRequestErrors.Get(httpErrorLabels{
				Label: label,
				Host:  r.Host,
				Code:  writer.statusCode,
			}).Add(1)
		}
		httpRequestBytesReturned.Get(labels).Put(float64(writer.responseSize(r)))
	})
}

// InstrumentHandlerFunc is identical to [InstrumentHandler] but takes a
// function instead of an http.Handler.
func InstrumentHandlerFunc(label string, f func(http.ResponseWriter, *http.Request)) http.Handler {
	return InstrumentHandler(label, http.HandlerFunc(f))
}

// responseWriterInstrumenter is a wrapper around an http.ResponseWriter that
// records the status code of the response.
type responseWriterInstrumenter struct {
	w          http.ResponseWriter
	statusCode int
	written    int // number of bytes written
}

var _ http.ResponseWriter = &responseWriterInstrumenter{}

// Header implements the http.ResponseWriter interface.
func (w *responseWriterInstrumenter) Header() http.Header {
	return w.w.Header()
}

// Write implements the http.ResponseWriter interface.
func (w *responseWriterInstrumenter) Write(b []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = 200
	}
	n, err := w.w.Write(b)
	w.written += n
	return n, err
}

// WriteHeader implements the http.ResponseWriter interface.
func (w *responseWriterInstrumenter) WriteHeader(statusCode int) {
	if w.statusCode == 0 {
		w.statusCode = statusCode
	}
	w.w.WriteHeader(statusCode)
}

// responseSize returns an approximation of the size, in bytes, of the HTTP
// response on the wire.
func (w *responseWriterInstrumenter) responseSize(req *http.Request) int {
	// An HTTP response looks something like this:
	//
	//     HTTP/1.1 200 OK
	//     Date: Wed, 09 Nov 2022 23:05:00 GMT
	//     Content-Length: 16
	//     Content-Type: text/plain; charset=utf-8
	//
	// There's a protocol (HTTP/1.1), a status code (200), a status (OK), a
	// header (Date: ...), and sometimes a body. We estimate the size of a
	// response by summing the sizes of these parts.
	size := 0
	size += len(req.Proto) // e.g., HTTP/1.1
	size += 3              // e.g., 200
	if w.statusCode == 0 {
		size += len(http.StatusText(200)) // e.g. OK
	} else {
		size += len(http.StatusText(w.statusCode)) // e.g., Not Found
	}
	for key, values := range w.Header() {
		for _, value := range values {
			size += len(key) + len(value) // e.g., Date: Wed, 09 Nov 2022 23:05:00 GMT
		}
	}
	size += w.written
	return size
}

// requestSize returns an approximation of the size, in bytes, of the HTTP
// request on the wire. If the size is unknown, requestSize returns false.
func requestSize(r *http.Request) (int, bool) {
	if r.ContentLength == -1 {
		// A ContentLength of -1 indicates an unknown size.
		return 0, false
	}

	// An HTTP request looks something like this:
	//
	//     GET /foo/bar?x=10 HTTP/1.1
	//     Host: localhost:35513
	//     User-Agent: curl/7.85.0
	//     Accept: */*
	//
	// There's a method (GET), a URL (/foo/bar?x=10), a protocol (HTTP/1.1), a
	// header (Host: ...), and sometimes a body. We estimate the size of a
	// request by summing the sizes of these parts.
	size := 0
	size += len(r.Method) // e.g., GET
	size += len(r.Proto)  // e.g., HTTP/1.1
	if r.URL != nil {
		size += len(r.URL.Path)     // e.g., /foo/bar
		size += len(r.URL.RawQuery) // e.g., ?x=10
		size += len(r.URL.Host)     // e.g., localhost:35513
	}
	for key, values := range r.Header {
		for _, value := range values {
			size += len(key) + len(value) // e.g., User-Agent: curl/7.85.0
		}
	}
	size += int(r.ContentLength)
	return size, true
}
