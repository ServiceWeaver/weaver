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

// Package main implements a service to compute the factors of an integer.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/ServiceWeaver/weaver"
)

//go:generate ../../cmd/weaver/weaver generate

func main() {
	flag.Parse()
	ctx := context.Background()
	if err := weaver.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
