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

package frontend

import (
	"net/http"
	"time"

	"github.com/google/uuid"
)

type responseRecorder struct {
	b      int
	status int
	w      http.ResponseWriter
}

var _ http.ResponseWriter = (*responseRecorder)(nil)

// Header implements the [http.ResponseWriter] interface.
func (r *responseRecorder) Header() http.Header {
	return r.w.Header()
}

// Write implements the [http.ResponseWriter] interface.
func (r *responseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.w.Write(p)
	r.b += n
	return n, err
}

// WriteHeader implements the [http.ResponseWriter] interface.
func (r *responseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.w.WriteHeader(statusCode)
}

type logHandler struct {
	server *server
	next   http.Handler
}

var _ http.Handler = (*logHandler)(nil)

func newLogHandler(s *server, next http.Handler) http.Handler {
	return &logHandler{server: s, next: next}
}

// ServeHTTP implements the [http.Handler] interface.
func (lh *logHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID, err := uuid.NewRandom()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	start := time.Now()
	rr := &responseRecorder{w: w}

	logger := lh.server.Logger(ctx).With(
		"http.req.path", r.URL.Path,
		"http.req.method", r.Method,
		"http.req.id", requestID)
	logger.Debug("request started")
	defer func() {
		logger.Debug("request complete",
			"duration", time.Since(start),
			"status", rr.status,
			"bytes", rr.b,
		)
	}()

	r = r.WithContext(ctx)
	lh.next.ServeHTTP(rr, r)
}
