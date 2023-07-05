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
	"sync"
	"testing"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestIncExport(t *testing.T) {
	clear()

	var exporter Exporter
	var importer Importer
	importExport := func() {
		if _, err := importer.Import(exporter.Export()); err != nil {
			t.Fatal(err)
		}
	}

	// Initialize counter so we get its def
	counter := Register(counterType, "TestIncExport/counter", "", nil)
	counter.Inc()
	importExport()
	if v := importer.metrics[counter.id].Value; v != 1 {
		t.Fatalf("expecting counter %v, got %v", v, 1)
	}

	// Export after Inc should generate an update.
	counter.Inc()
	importExport()
	if v := importer.metrics[counter.id].Value; v != 2 {
		t.Fatalf("expecting counter %v, got %v", v, 2)
	}
}

func TestExportImport(t *testing.T) {
	clear()
	var exporter Exporter
	var importer Importer
	io := func() {
		if _, err := importer.Import(exporter.Export()); err != nil {
			t.Fatal(err)
		}
	}

	counter := Register(counterType, "TestExportImport/counter", "", nil)
	counter.Add(10)
	io()

	gauge := Register(gaugeType, "TestExportImport/gauge", "", nil)
	counter.Add(10)
	gauge.Set(100)
	io()

	histogram := Register(histogramType, "TestExportImport/histogram", "", nil)
	counter.Add(10)
	gauge.Set(101)
	histogram.Put(1000)
	io()

	type ab = struct{ A, B string }
	counterFamily := RegisterMap[ab](counterType, "TestExportImport/counter_family", "", nil)
	counter.Add(10)
	gauge.Set(102)
	histogram.Put(1000)
	counterFamily.Get(ab{"1", "1"}).Add(10)
	counterFamily.Get(ab{"2", "2"}).Add(20)
	io()

	gaugeFamily := RegisterMap[ab](gaugeType, "TestExportImport/gauge_family", "", nil)
	counter.Add(10)
	gauge.Set(103)
	histogram.Put(1000)
	gaugeFamily.Get(ab{"1", "1"}).Set(100)
	gaugeFamily.Get(ab{"2", "2"}).Set(200)
	io()

	histogramFamily := RegisterMap[ab](histogramType, "TestExportImport/histogram_family", "", nil)
	counter.Add(10)
	gauge.Set(104)
	histogram.Put(1000)
	histogramFamily.Get(ab{"1", "1"}).Put(1000)
	histogramFamily.Get(ab{"2", "2"}).Put(2000)
	io()

	gauge.Set(105)
	histogram.Put(1000)
	counterFamily.Get(ab{"2", "2"}).Add(20)
	gaugeFamily.Get(ab{"1", "1"}).Set(101)
	histogramFamily.Get(ab{"2", "2"}).Put(2000)
	io()

	counter.Add(10)
	gauge.Set(106)
	counterFamily.Get(ab{"1", "1"}).Add(10)
	gaugeFamily.Get(ab{"2", "2"}).Set(201)
	io()

	want := []*MetricSnapshot{
		// Counters.
		{
			Type:  protos.MetricType_COUNTER,
			Name:  "TestExportImport/counter",
			Value: 70.0,
		},
		{
			Type:   protos.MetricType_COUNTER,
			Name:   "TestExportImport/counter_family",
			Value:  20.0,
			Labels: map[string]string{"a": "1", "b": "1"},
		},
		{
			Type:   protos.MetricType_COUNTER,
			Name:   "TestExportImport/counter_family",
			Value:  40.0,
			Labels: map[string]string{"a": "2", "b": "2"},
		},

		// Gauges.
		{
			Type:  protos.MetricType_GAUGE,
			Name:  "TestExportImport/gauge",
			Value: 106.0,
		},
		{
			Type:   protos.MetricType_GAUGE,
			Name:   "TestExportImport/gauge_family",
			Value:  101.0,
			Labels: map[string]string{"a": "1", "b": "1"},
		},
		{
			Type:   protos.MetricType_GAUGE,
			Name:   "TestExportImport/gauge_family",
			Value:  201.0,
			Labels: map[string]string{"a": "2", "b": "2"},
		},

		// Histogram.
		{
			Type:   protos.MetricType_HISTOGRAM,
			Name:   "TestExportImport/histogram",
			Value:  5000.0,
			Bounds: nil,
			Counts: []uint64{5},
		},
		{
			Type:   protos.MetricType_HISTOGRAM,
			Name:   "TestExportImport/histogram_family",
			Value:  1000.0,
			Labels: map[string]string{"a": "1", "b": "1"},
			Bounds: nil,
			Counts: []uint64{1},
		},
		{
			Type:   protos.MetricType_HISTOGRAM,
			Name:   "TestExportImport/histogram_family",
			Value:  4000.0,
			Labels: map[string]string{"a": "2", "b": "2"},
			Bounds: nil,
			Counts: []uint64{2},
		},
	}
	opts := []cmp.Option{
		cmpopts.IgnoreFields(MetricSnapshot{}, "Id", "Help"),
		cmpopts.SortSlices(func(x, y *MetricSnapshot) bool {
			return x.Value < y.Value
		}),
	}
	got, err := importer.Import(exporter.Export())
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("bad snapshot (-want +got):\n%s", diff)
	}
}

func TestConcurrentExportImport(t *testing.T) {
	clear()
	var exporter Exporter
	var importer Importer

	counter := Register(counterType, "TestConcurrentExportImport/counter", "", nil)

	const n = 100000
	done := make(chan struct{})
	go func() {
		for i := 0; i < n; i++ {
			counter.Add(1.0)
		}
		close(done)
	}()

	var exported sync.WaitGroup
	exported.Add(1)
	go func() {
		defer exported.Done()
		for {
			select {
			case <-done:
				return

			default:
				if _, err := importer.Import(exporter.Export()); err != nil {
					panic(err)
				}
			}
		}
	}()

	exported.Wait()
	want := []*MetricSnapshot{
		{
			Type:  protos.MetricType_COUNTER,
			Name:  "TestConcurrentExportImport/counter",
			Value: n,
		},
	}
	opts := []cmp.Option{cmpopts.IgnoreFields(MetricSnapshot{}, "Id", "Help")}
	got, err := importer.Import(exporter.Export())
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("bad snapshot (-want +got):\n%s", diff)
	}
}
