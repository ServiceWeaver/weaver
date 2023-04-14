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

// Package weavertest provides a way to test Service Weaver components.
//
// Use [weavertest.Run] to exercise a set of components. For example,
// imagine we have a Reverser component with a Reverse method that
// reverses strings. To test the Reverser component, we can create a
// reverser_test.go file with the following contents.
//
//	func TestReverse(t *testing.T) {
//	    weavertest.Run(t, weavetest.Options{}, func(reverser Reverser) {
//		got, err := reverser.Reverse(ctx, "diaper drawer")
//		if err != nil {
//		    t.Fatal(err)
//		}
//		if want := "reward repaid"; got != want {
//		    t.Fatalf("got %q, want %q", got, want)
//		}
//	    })
//	}
//
// weavertest.Run is passed a weavertest.Options, which you can use to configure
// the execution of the test. By default, weavertest.Run will run every
// component in a different process. This is similar to what happens when you
// run weaver multi deploy. If you set the SingleProcess option, weavertest.Run
// will instead run every component in a single process, similar to what
// happens when you "go run" a Service Weaver application.
//
//	func TestReverseSingle(t *testing.T) {
//	    weavertest.Run(t, weavetest.Options{SingleProcess: true}, func(reverser Reverser) {
//		// ...
//	    })
//	}
package weavertest
