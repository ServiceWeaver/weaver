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

// Package metrics provides an API for counters, gauges, and histograms.
//
// # Metric Types
//
// The metrics package provides types for three metrics: counters, gauges, and
// histograms.
//
//   - A [Counter] is a number that can only increase over time. It never
//     decreases. You can use a counter to measure things like the number of
//     HTTP requests your program has processed so far.
//   - A [Gauge] is a number that can increase or decrease over time. You can use
//     a gauge to measure things like the current amount of memory your program
//     is using, in bytes.
//   - A [Histogram] is a collection of numbers that are grouped into buckets.
//     You can use a histogram to measure things like the latency of every HTTP
//     request your program has received so far.
//
// # Declaring Metrics
//
// Declare metrics using [NewCounter], [NewGauge], or [NewHistogram]. We
// recommend you declare metrics at package scope.
//
//	var (
//		exampleCount = metrics.NewCounter(
//			"example_count",
//			"An example counter",
//		)
//		exampleGauge = metrics.NewGauge(
//			"example_gauge",
//			"An example gauge",
//		)
//		exampleHistogram = metrics.NewHistogram(
//			"example_histogram",
//			"An example histogram",
//			[]float64{1, 10, 100, 1000, 10000},
//		)
//	)
//
// [NonNegativeBuckets] returns a default set of non-negative buckets. It is
// useful when declaring histograms that measure things like memory or latency.
//
//	var exampleLatency = metrics.NewHistogram(
//		"example_latency",
//		"The latency of something, in microseconds",
//		metrics.NonNegativeBuckets,
//	)
//
// # Updating Metrics
//
// Every metric type has a set of methods you can use to update the metric.
// Counters can be added to. Gauges can be set. Histograms can have values
// added to them.
//
//	func main() {
//		exampleCount.Add(1)
//		exampleGauge.Set(2)
//		exampleHistogram.Put(3)
//	}
//
// # Metric Labels
//
// You can declare a metric with a set of key-value labels. For example, if you
// have a metric that measures the latency of handling HTTP requests, you can
// declare the metric with a label that identifies the URL of the request
// (e.g., "/users/foo").
//
// You can declare a labeled metric using [NewCounterMap], [NewGaugeMap], or
// [NewHistogramMap]. Service Weaver represents metric labels using structs.
// For example, here's how to declare a counter with labels "foo" and "bar":
//
//	type labels struct {
//	    Foo string
//	    Bar string
//	}
//	var exampleLabeledCounter = weaver.NewCounterMap[labels](
//	    "example_labeled_counter",
//	    `An example counter with labels "foo" and "bar"`,
//	)
//
// Use the Get method to get a metric with the provided set of label values.
//
//	func main() {
//	    // Get the counter with foo="a" and bar="b".
//	    counter := exampleLabeledCounter.Get(labels{Foo: "a", Bar: "b"})
//	    counter.Add(1)
//	}
//
// More precisely, labels are represented as a comparable struct of type L. We
// say a label struct L is valid if every field of L is a string, bool, or
// integer type and is exported. For example, the following are valid label
// structs:
//
//	struct{}         // an empty struct
//	struct{X string} // a struct with one field
//	struct{X, Y int} // a struct with two fields
//
// The following are invalid label structs:
//
//	struct{x string}   // unexported field
//	string             // not a struct
//	struct{X chan int} // unsupported label type (i.e. chan int)
//
// Note that the order of the fields within a label struct is unimportant. For
// example, if one program exports a metric with labels
//
//	struct{X, Y string}
//
// another program can safely export the same metric with labels
//
//	struct{Y, X string}
//
// To adhere to popular metric naming conventions, the first letter of every
// label is lowercased by default. The Foo label for example is exported as
// "foo", not "Foo". You can override this behavior and provide a custom label
// name using a weaver annotation.
//
//	type labels struct {
//	    Foo string                           // exported as "foo"
//	    Bar string `weaver:"my_custom_name"` // exported as "my_custom_name"
//	}
//
// # Exporting Metrics
//
// Service Weaver integrates metrics into the environment where your
// application is deployed. If you deploy a Service Weaver application to
// Google Cloud, for example, metrics are automatically exported to the Google
// Cloud Metrics Explorer where they can be queried, aggregated, and graphed.
// Refer to your deployer documentation for details.
package metrics
