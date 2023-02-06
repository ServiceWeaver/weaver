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

package main

import (
	"context"
	"testing"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/weavertest"
)

func TestDivergence(t *testing.T) {
	for _, test := range []struct {
		name   string
		single bool
		f      func(context.Context, weaver.Instance) (bool, error)
		want   bool
	}{
		{"Single/ConstructorErrors", true, constructorErrors, true},
		{"Single/Dealiasing", true, dealiasing, true},
		{"Single/CustomErrors", true, customErrors, true},

		{"Multi/ConstructorErrors", false, constructorErrors, false},
		{"Multi/Dealiasing", false, dealiasing, false},
		{"Multi/CustomErrors", false, customErrors, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			root := weavertest.Init(ctx, t, weavertest.Options{SingleProcess: test.single})
			got, err := test.f(ctx, root)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}
