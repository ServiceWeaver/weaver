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

	"github.com/ServiceWeaver/weaver/runtime"
)

// initSingleProcessLocal initializes a brand new single-process execution environment.
//
// config contains configuration identical to what might be found in a file passed
// when deploying an application. It can contain application level as well as
// component level configs. config is allowed to be empty.
func initSingleProcessLocal(ctx context.Context, config string) context.Context {
	return context.WithValue(ctx, runtime.BootstrapKey{}, runtime.Bootstrap{
		TestConfig: config,
		Quiet:      !testing.Verbose(),
	})
}
