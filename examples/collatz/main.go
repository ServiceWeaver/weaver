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

// Package main implements a service that explores the Collatz conjecture.
//
// Given a positive integer x, the Collatz process is the process of repeatedly
// executing the following operation:
//
//   - If x is even, set x to x/2.
//   - If x is odd, set x to 3x+1.
//
// For example, if we execute the Collatz process on x = 10, then we get the
// sequence of numbers 10, 5, 16, 8, 4, 2, 1. This sequence of numbers is
// called the hailstone sequence of 10. The Collatz conjecture states that the
// hailstone sequence of every positive number reaches 1. Nobody knows if the
// Collatz conjecture is true; it is one of the most famous unsolved problems
// in mathematics.
//
// Package main implements a service that executes the Collatz process. You can
// send the service a positive number, and the server replies with that
// number's hailstone sequence.
//
// The service is implemented as an HTTP server that uses two Service Weaver components:
// Odd and Even. Odd has a method Do that takes in an odd number x and returns
// 3x+1. Even has a method Do that takes in an even number x and return x/2.
// When the HTTP server receives a number x, it repeatedly sends x to either
// Odd or Even---depending on whether x is odd or even---until x equals 1. It
// then sends back the hailstone sequence.
//
// This Service Weaver application is interesting because it demonstrates the benefits
// of colocation. Colocating the main component with the Odd and Even
// components improves performance considerably.
//
// [1]: https://en.wikipedia.org/wiki/Collatz_conjecture
package main

import (
	"context"
	"flag"
	"log"

	"github.com/ServiceWeaver/weaver"
)

//go:generate ../../cmd/weaver/weaver generate

func main() {
	flag.Parse()
	if err := weaver.Run(context.Background(), serve); err != nil {
		log.Fatal(err)
	}
}
