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
	"github.com/ServiceWeaver/weaver/runtime/metrics"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

var (
	// Unlabeled counter.
	catCounter = metrics.Register(
		protos.MetricType_COUNTER,
		"example_cats",
		"Number of cats.",
		nil,
	)

	// Labeled counters.
	dogCounters = metrics.RegisterMap[dogLabels](
		protos.MetricType_COUNTER,
		"example_dogs",
		"Number of dogs, by breed.",
		nil,
	)
	corgiCounter     = dogCounters.Get(dogLabels{"corgi"})
	poodleCounter    = dogCounters.Get(dogLabels{"poodle"})
	dachshundCounter = dogCounters.Get(dogLabels{"dachshund"})
	dalmatianCounter = dogCounters.Get(dogLabels{"dalmatians"})
)

type dogLabels struct {
	Breed string
}

func Example() {
	catCounter.Add(9.0)
	corgiCounter.Add(2.0)
	poodleCounter.Add(1.0)
	dachshundCounter.Add(10.0)
	dalmatianCounter.Add(101.0)
}
