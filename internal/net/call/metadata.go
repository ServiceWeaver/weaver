// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package call

import (
	"context"

	"github.com/ServiceWeaver/weaver/metadata"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

// writeContextMetadata serializes the context metadata (if any) into enc.
func writeContextMetadata(ctx context.Context, enc *codegen.Encoder) {
	m, found := metadata.FromContext(ctx)
	if !found {
		enc.Bool(false)
		return
	}
	enc.Bool(true)
	enc.Len(len(m))
	for k, v := range m {
		enc.String(k)
		enc.String(v)
	}
}

// readContextMetadata returns the context metadata (if any) stored in dec.
func readContextMetadata(ctx context.Context, dec *codegen.Decoder) context.Context {
	hasMeta := dec.Bool()
	if !hasMeta {
		return ctx
	}
	n := dec.Len()
	res := make(map[string]string, n)
	var k, v string
	for i := 0; i < n; i++ {
		k = dec.String()
		v = dec.String()
		res[k] = v
	}
	return metadata.NewContext(ctx, res)
}
