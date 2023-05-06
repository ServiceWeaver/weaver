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

// testprogram is used by bin tests.
package main

import "github.com/ServiceWeaver/weaver"

//go:generate ../../../cmd/weaver/weaver generate

type A interface{}
type B interface{}
type C interface{}

type app struct {
	weaver.Implements[weaver.Main]
	a weaver.Ref[A] //nolint:unused // intentionally declared but not used
}

type a struct {
	weaver.Implements[A]
	b weaver.Ref[B] //nolint:unused // intentionally declared but not used
	c weaver.Ref[C] //nolint:unused // intentionally declared but not used
}

type b struct {
	weaver.Implements[B]
}

type c struct {
	weaver.Implements[C]
}

func main() {}
