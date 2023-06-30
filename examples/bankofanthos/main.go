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

// Package main implements a demo banking application called Bank of Anthos.
//
// This application is a forked version of Google Cloud's Bank of Anthos
// app [1], with the following changes:
//   - It is written entirely in Go.
//   - It is written as a single Service Weaver application.
//   - It is written to use Service Weaver specific logging/tracing/monitoring.
//
// [1]: https://github.com/GoogleCloudPlatform/bank-of-anthos
package main

import (
	"context"
	"flag"
	"log"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/frontend"
)

//go:generate ../../cmd/weaver/weaver generate

func main() {
	flag.Parse()
	if err := weaver.Run(context.Background(), frontend.Serve); err != nil {
		log.Fatal(err)
	}
}
