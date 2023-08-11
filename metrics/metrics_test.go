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

package metrics_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"golang.org/x/exp/maps"

	"github.com/ServiceWeaver/weaver/metrics"
	imetrics "github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

func ExampleCounterMap() {
	// At package scope.
	type carLabels struct {
		Make  string
		Model string
	}

	var (
		carCounts  = metrics.NewCounterMap[carLabels]("car_count", "The number of cars")
		civicCount = carCounts.Get(carLabels{"Honda", "Civic"})
		camryCount = carCounts.Get(carLabels{"Toyota", "Camry"})
		f150Count  = carCounts.Get(carLabels{"Ford", "F-150"})
	)

	// In your code.
	civicCount.Add(1)
	camryCount.Add(2)
	f150Count.Add(150)
}

func ExampleGaugeMap() {
	// At package scope.
	type tempLabels struct {
		Room  string
		Index int
	}

	var (
		temps          = metrics.NewGaugeMap[tempLabels]("temperature", "Temperature, in Fahrenheit")
		livingRoomTemp = temps.Get(tempLabels{"Living Room", 0})
		bathroomTemp   = temps.Get(tempLabels{"Bathroom", 0})
		bedroom0Temp   = temps.Get(tempLabels{"Bedroom", 0})
		bedroom1Temp   = temps.Get(tempLabels{"Bedroom", 1})
	)

	// In your code.
	livingRoomTemp.Set(78.4)
	bathroomTemp.Set(65.3)
	bedroom0Temp.Set(84.1)
	bedroom1Temp.Set(80.9)
	livingRoomTemp.Add(1.2)
	bathroomTemp.Sub(2.1)
}

func ExampleHistogramMap() {
	// At package scope.
	type latencyLabels struct {
		Endpoint string
	}

	var (
		latencies = metrics.NewHistogramMap[latencyLabels](
			"http_request_latency_nanoseconds",
			"HTTP request latency, in nanoseconds, by endpoint",
			[]float64{10, 100, 1000, 10000, 100000, 1000000, 10000000, 100000000},
		)
		fooLatency     = latencies.Get(latencyLabels{"/foo"})
		barLatency     = latencies.Get(latencyLabels{"/bar"})
		metricsLatency = latencies.Get(latencyLabels{"/metrics"})
	)

	// In your code.
	fooLatency.Put(8714.5)
	barLatency.Put(491.7)
	metricsLatency.Put(375.0)
}

func expect(t *testing.T, want *imetrics.MetricSnapshot) {
	getValue := func() *imetrics.MetricSnapshot {
		for _, m := range imetrics.Snapshot() {
			if m.Name == want.Name && maps.Equal(want.Labels, m.Labels) {
				return m
			}
		}
		return nil
	}
	got := getValue()
	opt := cmpopts.IgnoreFields(imetrics.MetricSnapshot{}, "Id")
	if diff := cmp.Diff(want, got, opt); diff != "" {
		t.Errorf("metric mismatch: (-want +got):\n%s", diff)
	}
}

func TestCounter(t *testing.T) {
	c := metrics.NewCounter(uuid.New().String(), "")
	c.Add(42)
	expect(t, &imetrics.MetricSnapshot{
		Type:  protos.MetricType_COUNTER,
		Name:  c.Name(),
		Value: 42,
	})
}

func TestCounterMap(t *testing.T) {
	type labels struct{ A, B, C string }
	c := metrics.NewCounterMap[labels](uuid.New().String(), "")
	c.Get(labels{"1", "2", "3"}).Add(42)
	c.Get(labels{"1", "2", "3"}).Add(42)
	c.Get(labels{"yi", "er", "san"}).Add(9001)
	expect(t, &imetrics.MetricSnapshot{
		Type:   protos.MetricType_COUNTER,
		Name:   c.Name(),
		Labels: map[string]string{"a": "1", "b": "2", "c": "3"},
		Value:  84,
	})
	expect(t, &imetrics.MetricSnapshot{
		Type:   protos.MetricType_COUNTER,
		Name:   c.Name(),
		Labels: map[string]string{"a": "yi", "b": "er", "c": "san"},
		Value:  9001,
	})
}

func TestGauge(t *testing.T) {
	g := metrics.NewGauge(uuid.New().String(), "")
	g.Set(42)
	g.Add(42)
	g.Sub(42)
	expect(t, &imetrics.MetricSnapshot{
		Type:  protos.MetricType_GAUGE,
		Name:  g.Name(),
		Value: 42,
	})
}

func TestGaugeMap(t *testing.T) {
	type labels struct{ A, B, C string }
	g := metrics.NewGaugeMap[labels](uuid.New().String(), "")
	g.Get(labels{"1", "2", "3"}).Set(42)
	g.Get(labels{"1", "2", "3"}).Add(42)
	g.Get(labels{"yi", "er", "san"}).Sub(9001)
	expect(t, &imetrics.MetricSnapshot{
		Type:   protos.MetricType_GAUGE,
		Name:   g.Name(),
		Labels: map[string]string{"a": "1", "b": "2", "c": "3"},
		Value:  84,
	})
	expect(t, &imetrics.MetricSnapshot{
		Type:   protos.MetricType_GAUGE,
		Name:   g.Name(),
		Labels: map[string]string{"a": "yi", "b": "er", "c": "san"},
		Value:  -9001,
	})

}

func TestHistogram(t *testing.T) {
	bounds := []float64{1, 10, 20}
	h := metrics.NewHistogram(uuid.New().String(), "", bounds)
	h.Put(42)
	expect(t, &imetrics.MetricSnapshot{
		Type:   protos.MetricType_HISTOGRAM,
		Name:   h.Name(),
		Value:  42,
		Bounds: bounds,
		Counts: []uint64{0, 0, 0, 1},
	})
}

func TestHistogramMap(t *testing.T) {
	name := uuid.New().String()
	bounds := []float64{1, 10, 20}
	type labels struct{ A, B, C string }
	c := metrics.NewHistogramMap[labels](name, "", bounds)
	c.Get(labels{"1", "2", "3"}).Put(42)
	c.Get(labels{"1", "2", "3"}).Put(42)
	c.Get(labels{"yi", "er", "san"}).Put(9001)
	expect(t, &imetrics.MetricSnapshot{
		Type:   protos.MetricType_HISTOGRAM,
		Name:   name,
		Labels: map[string]string{"a": "1", "b": "2", "c": "3"},
		Value:  84,
		Bounds: bounds,
		Counts: []uint64{0, 0, 0, 2},
	})
	expect(t, &imetrics.MetricSnapshot{
		Type:   protos.MetricType_HISTOGRAM,
		Name:   name,
		Labels: map[string]string{"a": "yi", "b": "er", "c": "san"},
		Value:  9001,
		Bounds: bounds,
		Counts: []uint64{0, 0, 0, 1},
	})
}
