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

package weaver_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ServiceWeaver/weaver"
	core "github.com/ServiceWeaver/weaver/internal/weaver"
)

type impl struct {
	A weaver.Ref[int]
	B weaver.Ref[string]
	C weaver.Ref[bool]
}

func getValue(t reflect.Type) (any, error) {
	if t == reflect.TypeOf(int(0)) {
		return 42, nil
	}
	if t == reflect.TypeOf("") {
		return "hello", nil
	}
	return nil, fmt.Errorf("unsupported type %v", t)
}

func TestFillRefs(t *testing.T) {
	var x struct {
		a weaver.Ref[int]
		b weaver.Ref[string]
	}
	if err := core.FillRefs(&x, getValue); err != nil {
		t.Fatal(err)
	}
	if x.a.Get() != 42 {
		t.Errorf("expecting x.a to be 42, got %d", x.a.Get())
	}
	if x.b.Get() != "hello" {
		t.Errorf("expecting x.b to be `hello`, got %s", x.b.Get())
	}
}

func TestFillRefsErrors(t *testing.T) {
	type badref struct {
		weaver.Ref[bool]
	}
	type testCase struct {
		name   string
		impl   any    // impl argument to pass to fillRefs
		expect string // Returned error must contain this string
	}
	for _, c := range []testCase{
		{"not-pointer", impl{}, "not a pointer"},
		{"not-struct-pointer", new(int), "not a struct pointer"},
		{"unsupported-type", &badref{}, "unsupported"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := core.FillRefs(c.impl, getValue)
			if err == nil || !strings.Contains(err.Error(), c.expect) {
				t.Fatalf("unexpected error %v; expecting %s", err, c.expect)
			}
		})
	}
}
