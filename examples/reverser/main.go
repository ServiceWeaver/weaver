// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"context"
	_ "embed"
	"flag"
	"log"

	"github.com/ServiceWeaver/weaver"
)

//go:generate ../../cmd/weaver/weaver generate

var (
	address = flag.String("address", ":9000", "Reverser server local address")

	//go:embed index.html
	indexHtml string // index.html served on "/"
)

func main() {
	// Initialize the Service Weaver application.
	flag.Parse()
	if err := weaver.Run(context.Background(), serve); err != nil {
		log.Fatal(err)
	}
}
