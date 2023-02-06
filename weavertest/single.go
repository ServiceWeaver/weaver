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
	"fmt"
	"os"
	rt "runtime"
	"testing"
	"time"

	weaver "github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/runtime"
)

// initSingleProcess initializes a brand new single-process execution environment and
// returns the root component for the new application.
//
// config contains configuration identical to what might be found in a file passed
// when deploying an application. It can contain application level as well as
// component level configs. config is allowed to be empty.
func initSingleProcess(ctx context.Context, t testing.TB, config string) weaver.Instance {
	t.Helper()
	ctx, cancelFunc := context.WithCancel(ctx)
	t.Cleanup(func() {
		cancelFunc()
		maybeLogStacks()
	})
	ctx = context.WithValue(ctx, runtime.BootstrapKey{}, runtime.Bootstrap{
		TestConfig: config,
	})
	return weaver.Init(ctx)
}

func maybeLogStacks() {
	// Disable early return to find background work that is not obeying cancellation.
	if true {
		return
	}

	time.Sleep(time.Second) // Hack to wait for goroutines to end
	buf := make([]byte, 1<<20)
	n := rt.Stack(buf, true)
	fmt.Fprintf(os.Stderr, "%s\n", string(buf[:n]))
}
