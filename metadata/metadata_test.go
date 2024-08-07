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

package metadata

import (
	"context"
	"reflect"
	"testing"
)

func TestContextMetadata(t *testing.T) {
	type testCase struct {
		name           string
		meta           map[string]string
		isMetaExpected bool
		want           map[string]string
	}
	for _, test := range []testCase{
		{
			name: "no metadata",
		},
		{
			name:           "with empty metadata",
			meta:           map[string]string{},
			isMetaExpected: false,
		},
		{
			name: "with valid metadata",
			meta: map[string]string{
				"foo": "bar",
				"baz": "waldo",
			},
			isMetaExpected: true,
			want: map[string]string{
				"foo": "bar",
				"baz": "waldo",
			},
		},
		{
			name: "with valid metadata and uppercase keys",
			meta: map[string]string{
				"Foo": "bar",
				"Baz": "waldo",
			},
			isMetaExpected: true,
			want: map[string]string{
				"Foo": "bar",
				"Baz": "waldo",
			},
		},
		{
			name: "with valid metadata and uppercase values",
			meta: map[string]string{
				"Foo": "Bar",
				"Baz": "Waldo",
			},
			isMetaExpected: true,
			want: map[string]string{
				"Foo": "Bar",
				"Baz": "Waldo",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			if len(test.meta) > 0 {
				ctx = NewContext(ctx, test.meta)
			}

			got, found := FromContext(ctx)
			if !reflect.DeepEqual(found, test.isMetaExpected) {
				t.Errorf("ExtractMetadata: expecting %v, got %v", test.isMetaExpected, found)
			}
			if !found {
				return
			}
			if !reflect.DeepEqual(test.want, got) {
				t.Errorf("ExtractMetadata: expecting %v, got %v", test.want, got)
			}
		})
	}
}
