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

package status

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"

	"github.com/ServiceWeaver/weaver/runtime/metrics"
	imetrics "github.com/ServiceWeaver/weaver/runtime/prometheus"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	protos "github.com/ServiceWeaver/weaver/runtime/protos"
)

const (
	statusEndpoint     = "/debug/serviceweaver/status"
	metricsEndpoint    = "/debug/serviceweaver/metrics"
	prometheusEndpoint = "/debug/serviceweaver/prometheus"
	profileEndpoint    = "/debug/serviceweaver/profile"
)

// A Server returns information about a Service Weaver deployment.
type Server interface {
	// Status returns the status of the deployment.
	Status(context.Context) (*Status, error)

	// Metrics returns a snapshot of the deployment's metrics.
	Metrics(context.Context) (*Metrics, error)

	// Profile returns a profile of the deployment.
	Profile(context.Context, *protos.GetProfileRequest) (*protos.GetProfileReply, error)
}

// RegisterServer registers a Server's methods with the provided mux under the
// /debug/serviceweaver/ prefix. You can use a Client to interact with a Status server.
func RegisterServer(mux *http.ServeMux, server Server, logger *slog.Logger) {
	mux.Handle(statusEndpoint, protomsg.HandlerThunk(logger, server.Status))
	mux.Handle(metricsEndpoint, protomsg.HandlerThunk(logger, server.Metrics))
	mux.Handle(profileEndpoint, protomsg.HandlerFunc(logger, server.Profile))
	mux.HandleFunc(prometheusEndpoint, func(w http.ResponseWriter, r *http.Request) {
		ms, err := server.Metrics(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		snapshots := make([]*metrics.MetricSnapshot, len(ms.Metrics))
		for i, m := range ms.Metrics {
			snapshots[i] = metrics.UnProto(m)
		}
		var b bytes.Buffer
		imetrics.TranslateMetricsToPrometheusTextFormat(&b, snapshots, r.Host, prometheusEndpoint)
		w.Write(b.Bytes())
	})
}
