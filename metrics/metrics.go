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

package metrics

import (
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// A Counter is a float-valued, monotonically increasing metric. For example,
// you can use a Counter to count the number of requests received, the number
// of requests that resulted in an error, etc.
type Counter struct {
	impl *metrics.Metric
}

// NewCounter returns a new Counter.
// It is typically called during package initialization since it
// panics if called more than once in the same process with the same name.
// Use NewCounterMap to make a Counter with labels.
func NewCounter(name, help string) *Counter {
	return &Counter{impl: metrics.Register(protos.MetricType_COUNTER, name, help, nil)}
}

// Name returns the name of the counter.
func (c *Counter) Name() string {
	return c.impl.Name()
}

// Inc increases the counter by one.
func (c *Counter) Inc() {
	c.impl.Inc()
}

// Add increases the counter by delta. It panics if the delta is negative.
func (c *Counter) Add(delta float64) {
	c.impl.Add(delta)
}

// A CounterMap is a collection of Counters with the same name and label schema
// but with different label values. See package documentation for a description
// of L.
type CounterMap[L comparable] struct {
	impl *metrics.MetricMap[L]
}

// NewCounterMap returns a new CounterMap.
// It is typically called during package initialization since it
// panics if called more than once in the same process with the same name.
func NewCounterMap[L comparable](name, help string) *CounterMap[L] {
	return &CounterMap[L]{metrics.RegisterMap[L](protos.MetricType_COUNTER, name, help, nil)}
}

// Name returns the name of the CounterMap.
func (c *CounterMap[L]) Name() string {
	return c.impl.Name()
}

// Get returns the Counter with the provided labels, constructing it if it
// doesn't already exist. Multiple calls to Get with the same labels will
// return the same Counter.
func (c *CounterMap[L]) Get(labels L) *Counter {
	return &Counter{c.impl.Get(labels)}
}

// A Gauge is a float-valued metric that can hold an arbitrary numerical value,
// which can increase or decrease over time. For example, you can use a Gauge
// to measure the current memory usage or the current number of outstanding
// requests.
type Gauge struct {
	impl *metrics.Metric
}

// NewGauge returns a new Gauge.
// It is typically called during package initialization since it
// panics if called more than once in the same process with the same name.
// Use NewGaugeMap to make a Gauge with labels.
func NewGauge(name, help string) *Gauge {
	return &Gauge{impl: metrics.Register(protos.MetricType_GAUGE, name, help, nil)}
}

// Name returns the name of the Gauge.
func (g *Gauge) Name() string {
	return g.impl.Name()
}

// Set sets the gauge to the given value, overwriting any previous value.
func (g *Gauge) Set(val float64) {
	g.impl.Set(val)
}

// Add adds the provided delta to the gauge's value.
func (g *Gauge) Add(delta float64) {
	g.impl.Add(delta)
}

// Sub subtracts the provided delta from the gauge's value.
func (g *Gauge) Sub(delta float64) {
	g.impl.Sub(delta)
}

// A GaugeMap is a collection of Gauges with the same name and label schema but
// with different label values. See package documentation for a description of L.
type GaugeMap[L comparable] struct {
	impl *metrics.MetricMap[L]
}

// NewGaugeMap returns a new GaugeMap.
// It is typically called during package initialization since it
// panics if called more than once in the same process with the same name.
func NewGaugeMap[L comparable](name, help string) *GaugeMap[L] {
	return &GaugeMap[L]{metrics.RegisterMap[L](protos.MetricType_GAUGE, name, help, nil)}
}

// Name returns the name of the GaugeMap.
func (g *GaugeMap[L]) Name() string {
	return g.impl.Name()
}

// Get returns the Gauge with the provided labels, constructing it if it
// doesn't already exist. Multiple calls to Get with the same labels will
// return the same Gauge.
func (g *GaugeMap[L]) Get(labels L) *Gauge {
	return &Gauge{g.impl.Get(labels)}
}

// A Histogram is a metric that counts the number of values that fall in
// specified ranges (i.e. buckets). For example, you can use a Histogram to
// measure the distribution of request latencies.
type Histogram struct {
	impl *metrics.Metric
}

// NewHistogram returns a new Histogram.
// It is typically called during package initialization since it
// panics if called more than once in the same process with the same name.
// Use NewHistogram to make a Histogram with labels.
//
// The bucket boundaries must be strictly increasing. Given n boundary values,
// the histogram will contain n+1 buckets, organized as follows:
//
//   - bucket[0] is the underflow bucket, which counts values in the range
//     [-inf, bounds[0]).
//   - bucket[n] is the overflow bucket, which counts values in the range
//     [bounds[n-1], +inf).
//   - bucket[i], for 0 < i < n, is a bucket that counts values in the range
//     [bounds[i-1], bounds[i]).
//
// For example, given the bounds [0, 10, 100], we have the following buckets:
//
//   - Bucket 0: (-inf, 0]
//   - Bucket 1: [0, 10)
//   - Bucket 2: [10, 100)
//   - Bucket 3: [100, +inf)
func NewHistogram(name, help string, bounds []float64) *Histogram {
	return &Histogram{impl: metrics.Register(protos.MetricType_HISTOGRAM, name, help, bounds)}
}

// Name returns the name of the histogram.
func (h *Histogram) Name() string {
	return h.impl.Name()
}

// Put records a value in its bucket.
func (h *Histogram) Put(val float64) {
	h.impl.Put(val)
}

// A HistogramMap is a collection of Histograms with the same name and label
// schema but with different label values. See package documentation for a
// description of L.
type HistogramMap[L comparable] struct {
	impl *metrics.MetricMap[L]
}

// NewHistogramMap returns a new HistogramMap.
// It is typically called during package initialization since it
// panics if called more than once in the same process with the same name.
func NewHistogramMap[L comparable](name, help string, bounds []float64) *HistogramMap[L] {
	return &HistogramMap[L]{metrics.RegisterMap[L](protos.MetricType_HISTOGRAM, name, help, bounds)}
}

// Name returns the name of the HistogramMap.
func (h *HistogramMap[L]) Name() string {
	return h.impl.Name()
}

// Get returns the Histogram with the provided labels, constructing it if it
// doesn't already exist. Multiple calls to Get with the same labels will
// return the same Histogram.
func (h *HistogramMap[L]) Get(labels L) *Histogram {
	return &Histogram{h.impl.Get(labels)}
}
