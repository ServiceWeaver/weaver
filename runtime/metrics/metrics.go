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

// Package metrics implements Service Weaver metrics.
package metrics

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

var (
	// metricNames stores the name of every metric (labeled or not).
	metricNamesMu sync.RWMutex
	metricNames   = map[string]bool{}

	// metrics stores every metric.
	metricsMu sync.RWMutex
	metrics   = []*Metric{}
)

// Metric is a thread-safe readable and writeable metric. It is the underlying
// implementation of the user-facing metrics like Counter and Gauge.
//
// Every metric has a unique name assigned by the user. For example, the user
// may create a histogram called "http_request_duration". Every metric also has
// a fixed, possibly empty, set of labels. For example, the user may assign an
// "endpoint" label to their "http_request_duration" to differentiate the
// latency of different HTTP endpoints. A metric name and set of label values
// uniquely identify a metric. For example, the following two metrics are
// different:
//
//	http_request_duration{endpoint="/"}
//	http_request_duration{endpoint="/foo"}
type Metric struct {
	typ         protos.MetricType        // the type of the metric
	name        string                   // the globally unique metric name
	help        string                   // a short description of the metric
	labelsThunk func() map[string]string // the (deferred) metric labels

	// Users may call Get on the critical path of their application, so we want
	// a call of `Get(labels)` to be as fast as possible. Converting `labels`
	// into a map[string]string requires reflection and can be slow. Computing
	// the metric's id is similarly slow. We avoid doing either of these in the
	// call to Get and instead initialize them only when needed (i.e. before
	// exporting).
	once   sync.Once         // used to initialize id and labels
	id     uint64            // globally unique metric id
	labels map[string]string // materialized labels from calling labelsThunk

	fvalue atomicFloat64 // value for Counter and Gauge, sum for Histogram
	ivalue atomic.Uint64 // integer increments for Counter (separated for speed)

	// For histograms only:
	putCount atomic.Uint64   // incremented on every Put, for change detection
	bounds   []float64       // histogram bounds
	counts   []atomic.Uint64 // histogram counts
}

// A MetricSnapshot is a snapshot of a metric.
type MetricSnapshot struct {
	Id     uint64
	Type   protos.MetricType
	Name   string
	Labels map[string]string
	Help   string

	Value  float64
	Bounds []float64
	Counts []uint64
}

// MetricDef returns a MetricDef derived from the metric.
func (m *MetricSnapshot) MetricDef() *protos.MetricDef {
	return &protos.MetricDef{
		Id:     m.Id,
		Name:   m.Name,
		Typ:    m.Type,
		Help:   m.Help,
		Labels: m.Labels,
		Bounds: m.Bounds,
	}
}

// MetricValue returns a MetricValue derived from the metric.
func (m *MetricSnapshot) MetricValue() *protos.MetricValue {
	return &protos.MetricValue{
		Id:     m.Id,
		Value:  m.Value,
		Counts: m.Counts,
	}
}

// MetricSnapshot converts a MetricSnapshot to its proto equivalent.
func (m *MetricSnapshot) ToProto() *protos.MetricSnapshot {
	return &protos.MetricSnapshot{
		Id:     m.Id,
		Name:   m.Name,
		Typ:    m.Type,
		Help:   m.Help,
		Labels: m.Labels,
		Bounds: m.Bounds,
		Value:  m.Value,
		Counts: m.Counts,
	}
}

// UnProto converts a protos.MetricSnapshot into a metrics.MetricSnapshot.
func UnProto(m *protos.MetricSnapshot) *MetricSnapshot {
	return &MetricSnapshot{
		Id:     m.Id,
		Type:   m.Typ,
		Name:   m.Name,
		Labels: m.Labels,
		Help:   m.Help,
		Value:  m.Value,
		Bounds: m.Bounds,
		Counts: m.Counts,
	}
}

// Clone returns a deep copy of m.
func (m *MetricSnapshot) Clone() *MetricSnapshot {
	c := *m
	c.Labels = maps.Clone(m.Labels)
	c.Bounds = slices.Clone(m.Bounds)
	c.Counts = slices.Clone(m.Counts)
	return &c
}

// config configures the creation of a metric.
type config struct {
	Type   protos.MetricType
	Name   string
	Labels func() map[string]string
	Bounds []float64
	Help   string
}

// Register registers and returns a new metric. Panics if a metric with the same name
// has already been registered.
func Register(typ protos.MetricType, name string, help string, bounds []float64) *Metric {
	m := RegisterMap[struct{}](typ, name, help, bounds)
	return m.Get(struct{}{})
}

// newMetric registers and returns a new metric.
func newMetric(config config) *Metric {
	metricsMu.Lock()
	defer metricsMu.Unlock()
	metric := &Metric{
		typ:         config.Type,
		name:        config.Name,
		help:        config.Help,
		labelsThunk: config.Labels,
		bounds:      config.Bounds,
	}
	if config.Type == protos.MetricType_HISTOGRAM {
		metric.counts = make([]atomic.Uint64, len(config.Bounds)+1)
	}
	metrics = append(metrics, metric)
	return metric
}

// Name returns the name of the metric.
func (m *Metric) Name() string {
	return m.name
}

// Inc adds one to the metric value.
func (m *Metric) Inc() {
	m.ivalue.Add(1)
}

// Add adds the provided delta to the metric's value.
func (m *Metric) Add(delta float64) {
	m.fvalue.add(delta)
}

// Sub subtracts the provided delta from the metric's value.
func (m *Metric) Sub(delta float64) {
	m.fvalue.add(-delta)
}

// Set sets the metric's value.
func (m *Metric) Set(val float64) {
	m.fvalue.set(val)
}

// Put adds the provided value to the metric's histogram.
func (m *Metric) Put(val float64) {
	var idx int
	if len(m.bounds) == 0 || val < m.bounds[0] {
		// Skip binary search for values that fall in the first bucket
		// (often true for short latency operations).
	} else {
		idx = sort.SearchFloat64s(m.bounds, val)
		if idx < len(m.bounds) && val == m.bounds[idx] {
			idx++
		}
	}
	m.counts[idx].Add(1)

	// Microsecond latencies are often zero for very fast functions.
	if val != 0 {
		m.fvalue.add(val)
	}
	m.putCount.Add(1)
}

// initIdAndLabels initializes the id and labels of a metric.
// We delay this initialization until the first time we export a
// metric to avoid slowing down a Get() call.
func (m *Metric) initIdAndLabels() {
	m.once.Do(func() {
		if labels := m.labelsThunk(); len(labels) > 0 {
			m.labels = labels
		}
		var id [16]byte = uuid.New()
		m.id = binary.LittleEndian.Uint64(id[:8])
	})
}

// get returns the current value (sum of all added values for histograms).
func (m *Metric) get() float64 {
	return m.fvalue.get() + float64(m.ivalue.Load())
}

// Snapshot returns a snapshot of the metric. You must call Init at least once
// before calling Snapshot.
func (m *Metric) Snapshot() *MetricSnapshot {
	var counts []uint64
	if n := len(m.counts); n > 0 {
		counts = make([]uint64, n)
		for i := range m.counts {
			counts[i] = m.counts[i].Load()
		}
	}
	return &MetricSnapshot{
		Id:     m.id,
		Name:   m.name,
		Type:   m.typ,
		Help:   m.help,
		Labels: maps.Clone(m.labels),
		Value:  m.get(),
		Bounds: slices.Clone(m.bounds),
		Counts: counts,
	}
}

// MetricDef returns a MetricDef derived from the metric. You must call Init at
// least once before calling Snapshot.
func (m *Metric) MetricDef() *protos.MetricDef {
	return &protos.MetricDef{
		Id:     m.id,
		Name:   m.name,
		Typ:    m.typ,
		Help:   m.help,
		Labels: maps.Clone(m.labels),
		Bounds: slices.Clone(m.bounds),
	}
}

// MetricValue returns a MetricValue derived from the metric.
func (m *Metric) MetricValue() *protos.MetricValue {
	var counts []uint64
	if n := len(m.counts); n > 0 {
		counts = make([]uint64, n)
		for i := range m.counts {
			counts[i] = m.counts[i].Load()
		}
	}
	return &protos.MetricValue{
		Id:     m.id,
		Value:  m.get(),
		Counts: counts,
	}
}

// MetricMap is a collection of metrics with the same name and label schema
// but with different label values. See public metric documentation for
// an explanation of labels.
//
// TODO(mwhittaker): Understand the behavior of prometheus and Google Cloud
// Metrics when we add or remove metric labels over time.
type MetricMap[L comparable] struct {
	config    config             // configures the metrics returned by Get
	extractor *labelExtractor[L] // extracts labels from a value of type L
	mu        sync.Mutex         // guards metrics
	metrics   map[L]*Metric      // cache of metrics, by label
}

func RegisterMap[L comparable](typ protos.MetricType, name string, help string, bounds []float64) *MetricMap[L] {
	if err := typecheckLabels[L](); err != nil {
		panic(err)
	}
	if name == "" {
		panic(fmt.Errorf("empty metric name"))
	}
	if typ == protos.MetricType_INVALID {
		panic(fmt.Errorf("metric %q: invalid metric type %v", name, typ))
	}
	for _, x := range bounds {
		if math.IsNaN(x) {
			panic(fmt.Errorf("metric %q: NaN histogram bound", name))
		}
	}
	for i := 0; i < len(bounds)-1; i++ {
		if bounds[i] >= bounds[i+1] {
			panic(fmt.Errorf("metric %q: non-ascending histogram bounds %v", name, bounds))
		}
	}

	metricNamesMu.Lock()
	defer metricNamesMu.Unlock()
	if metricNames[name] {
		panic(fmt.Errorf("metric %q already exists", name))
	}
	metricNames[name] = true
	return &MetricMap[L]{
		config:    config{Type: typ, Name: name, Help: help, Bounds: bounds},
		extractor: newLabelExtractor[L](),
		metrics:   map[L]*Metric{},
	}
}

// Name returns the name of the metricMap.
func (mm *MetricMap[L]) Name() string {
	return mm.config.Name
}

// Get returns the metric with the provided labels, constructing it if it
// doesn't already exist. Multiple calls to Get with the same labels will
// return the same metric.
func (mm *MetricMap[L]) Get(labels L) *Metric {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	if metric, ok := mm.metrics[labels]; ok {
		return metric
	}
	config := mm.config
	config.Labels = func() map[string]string {
		return mm.extractor.Extract(labels)
	}
	metric := newMetric(config)
	mm.metrics[labels] = metric
	return metric
}

// Snapshot returns a snapshot of all currently registered metrics. The
// snapshot is not guaranteed to be atomic.
func Snapshot() []*MetricSnapshot {
	metricsMu.RLock()
	defer metricsMu.RUnlock()
	snapshots := make([]*MetricSnapshot, 0, len(metrics))
	for _, metric := range metrics {
		metric.initIdAndLabels()
		snapshots = append(snapshots, metric.Snapshot())
	}
	return snapshots
}
