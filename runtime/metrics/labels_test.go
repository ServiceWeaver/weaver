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

package metrics

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// welltyped asserts that L is a valid label struct.
func welltyped[L comparable](t *testing.T) {
	t.Helper()
	if err := typecheckLabels[L](); err != nil {
		t.Error(err)
	}
}

// mistyped asserts that L is not a valid label struct and produces a
// typechecking error that contains the provided text.
func mistyped[L comparable](t *testing.T, want string) {
	t.Helper()
	err := typecheckLabels[L]()
	if err == nil {
		t.Error("unexpected success")
		return
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("bad error: got %q, want %q", err.Error(), want)
	}
}

func TestValidLabels(t *testing.T) {
	type t1 = struct{}
	type t2 = struct{ A string }
	type t3 = struct{ A bool }
	type t4 = struct {
		A int
		B int8
		C int16
		D int32
		E int64
	}
	type t5 = struct {
		A uint
		B uint8
		C uint16
		D uint32
		E uint64
	}
	type t6 = struct {
		X string `weaver:"x"`
		Y bool   `weaver:"y"`
	}
	type t7 = struct {
		X int  `weaver:"X"`
		Y int8 `weaver:"Y"`
	}
	type t8 = struct {
		X int16 `weaver:"Y"`
		Y int32 `weaver:"X"`
	}

	type u1 t1
	type u2 t2
	type u3 t3
	type u4 t4
	type u5 t5
	type u6 t6
	type u7 t7
	type u8 t8

	welltyped[t1](t)
	welltyped[t2](t)
	welltyped[t3](t)
	welltyped[t4](t)
	welltyped[t5](t)
	welltyped[t6](t)
	welltyped[t7](t)
	welltyped[t8](t)
	welltyped[u1](t)
	welltyped[u2](t)
	welltyped[u3](t)
	welltyped[u4](t)
	welltyped[u5](t)
	welltyped[u6](t)
	welltyped[u7](t)
	welltyped[u8](t)
}

func TestInvalidLabels(t *testing.T) {
	type t1 = string             // not a struct
	type t2 = struct{ x string } //lint:ignore U1000 unexported field
	type t3 = struct {           // duplicate field
		X string `weaver:"Z"`
		Y string `weaver:"Z"`
	}
	type t4 = struct { // duplicate field
		X string `weaver:"y"`
		Y string
	}
	type String string
	type Bool bool
	type t5 = struct{ X String }             // unsupported type
	type t6 = struct{ X Bool }               // unsupported type
	type t7 = struct{ X float32 }            // unsupported type
	type t8 = struct{ X struct{ Y string } } // unsupported type

	type u1 t1
	type u2 t2
	type u3 t3
	type u4 t4
	type u5 t5
	type u6 t6
	type u7 t7
	type u8 t8

	mistyped[t1](t, "not a struct")
	mistyped[t2](t, "unexported")
	mistyped[t3](t, "duplicate field")
	mistyped[t4](t, "duplicate field")
	mistyped[t5](t, "unsupported type")
	mistyped[t6](t, "unsupported type")
	mistyped[t7](t, "unsupported type")
	mistyped[t8](t, "unsupported type")

	mistyped[u1](t, "not a struct")
	mistyped[u2](t, "unexported")
	mistyped[u3](t, "duplicate field")
	mistyped[u4](t, "duplicate field")
	mistyped[u5](t, "unsupported type")
	mistyped[u6](t, "unsupported type")
	mistyped[u7](t, "unsupported type")
	mistyped[u8](t, "unsupported type")
}

func TestLabelExtractor(t *testing.T) {
	type labels struct {
		A string `weaver:"a"`
		B bool   `weaver:"C"`
		C int    `weaver:"B"`
		D uint
	}
	welltyped[labels](t)
	extractor := newLabelExtractor[labels]()
	got := extractor.Extract(labels{"a", true, 0, 1})
	want := map[string]string{"a": "a", "C": "true", "B": "0", "d": "1"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("bad label extraction (-want +got)\n%s", diff)
	}
}
