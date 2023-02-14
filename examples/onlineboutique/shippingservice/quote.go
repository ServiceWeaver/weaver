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

package shippingservice

import (
	"fmt"
	"math"
)

// Quote represents a currency value.
type quote struct {
	Dollars uint32
	Cents   uint32
}

// String representation of the Quote.
func (q quote) String() string {
	return fmt.Sprintf("$%d.%d", q.Dollars, q.Cents)
}

// createQuoteFromCount takes a number of items and returns a quote.
func createQuoteFromCount(count int) quote {
	return createQuoteFromFloat(8.99)
}

// createQuoteFromFloat takes a price represented as a float and creates a quote.
func createQuoteFromFloat(value float64) quote {
	units, fraction := math.Modf(value)
	return quote{
		uint32(units),
		uint32(math.Trunc(fraction * 100)),
	}
}
