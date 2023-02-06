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

package weavertest

import (
	"context"
	"testing"

	"github.com/ServiceWeaver/weaver"
)

// Options configure weavertest.Init.
type Options struct {
	// If true, every component is colocated in a single process. Otherwise,
	// every component is run in a separate OS process.
	SingleProcess bool

	// Config contains configuration identical to what might be found in a
	// Service Weaver config file. It can contain application level as well as component
	// level configuration. Config is allowed to be empty.
	Config string
}

// Init is a testing version of weaver.Init. Calling Init will create a brand
// new execution environment and return the main component. For example:
//
//	func TestFoo(t *testing.T) {
//	    root := weavertest.Init(context.Background(), t, weavertest.Options{})
//	    foo, err := weaver.Get[Foo](root)
//	    if err != nil {
//	        t.Fatal(err)
//	    }
//	    // Test the Foo component...
//	}
func Init(ctx context.Context, t testing.TB, opts Options) weaver.Instance {
	if opts.SingleProcess {
		return initSingleProcess(ctx, t, opts.Config)
	}
	return initMultiProcess(ctx, t, opts.Config)
}
