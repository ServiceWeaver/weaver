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

// Package metadata provides support for the propagation of metadata information
// from a component method caller to the callee. The metadata is propagated to
// the callee even if the caller and callee are not colocated in the same process.
//
// The metadata is a map from string to string stored in context.Context. The map
// can be added to a context by calling NewContext.
//
// Example:
//
// To attach metadata with key "foo" and value "bar" to the context:
//
//	ctx := context.Background()
//	ctx = metadata.NewContext(ctx, map[string]string{"foo": "bar"})
//
// To read the metadata value associated with a key "foo" in the context:
//
//	meta, found := metadata.FromContext(ctx)
//	if found {
//		  value := meta["foo"]
//	}
package metadata

import (
	"context"
	"maps"
)

// metaKey is an unexported type for the key that stores the metadata.
type metaKey struct{}

// NewContext returns a new context that carries metadata meta.
func NewContext(ctx context.Context, meta map[string]string) context.Context {
	return context.WithValue(ctx, metaKey{}, meta)
}

// FromContext returns the metadata value stored in ctx, if any.
func FromContext(ctx context.Context) (map[string]string, bool) {
	meta, ok := ctx.Value(metaKey{}).(map[string]string)
	if !ok {
		return nil, false
	}
	out := maps.Clone(meta)
	return out, true
}
