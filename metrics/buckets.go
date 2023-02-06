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

package metrics

// NonNegativeBuckets provides rounded bucket boundaries for histograms
// that will only store non-negative values.
var NonNegativeBuckets = []float64{
	// Adjacent buckets differ from each other by 2x or 2.5x.
	1, 2, 5,
	10, 20, 50,
	100, 200, 500,
	1000, 2000, 5000,
	10000, 20000, 50000,
	100000, 200000, 500000,
	1000000, 2000000, 5000000,
	10000000, 20000000, 50000000,
	100000000, 200000000, 500000000,
	1000000000, 2000000000, 5000000000,
	10000000000, 20000000000, 50000000000,
	100000000000, 200000000000, 500000000000,
	1000000000000, 2000000000000, 5000000000000,
	10000000000000, 20000000000000, 50000000000000,
	100000000000000, 200000000000000, 500000000000000,
	1000000000000000, 2000000000000000, 5000000000000000,
	10000000000000000, 20000000000000000, 50000000000000000,
	100000000000000000, 200000000000000000, 500000000000000000,
	1000000000000000000, 2000000000000000000, 5000000000000000000,
	10000000000000000000, 20000000000000000000, 50000000000000000000, // i.e., 5e19
}
