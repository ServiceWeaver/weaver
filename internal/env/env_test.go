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

package env

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func ExampleSplit() {
	key, value, err := Split("name=ava")
	if err != nil {
		panic(err)
	}
	fmt.Println(key)
	fmt.Println(value)
	// Output: name
	// ava
}

func ExampleParse() {
	vars, err := Parse([]string{
		"name=alice",
		"pet=cat",
		"name=bob", // bob overwrites alice
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(vars["name"])
	fmt.Println(vars["pet"])
	// Output: bob
	// cat
}

func TestSplit(t *testing.T) {
	for _, test := range []struct {
		s, k, v string
		good    bool
	}{
		{"foo=bar", "foo", "bar", true},
		{"foo=bar=baz", "foo", "bar=baz", true},
		{"foo=", "foo", "", true},
		{"=bar", "", "bar", true},
		{"", "", "", false},
		{"foo", "", "", false},
	} {
		t.Run(test.s, func(t *testing.T) {
			k, v, err := Split(test.s)
			if test.good && err != nil {
				t.Fatalf("env.Split(%q): %v", test.s, err)
			}
			if !test.good && err == nil {
				t.Fatalf("env.Split(%q): unexpected succeess", test.s)
			}
			if test.good && (k != test.k || v != test.v) {
				t.Fatalf("env.Split(%q): got (%q, %q), want (%q, %q)", test.s, k, v, test.k, test.v)
			}
		})
	}
}

func TestParse(t *testing.T) {
	for _, test := range []struct {
		env  []string
		want map[string]string
	}{
		{[]string{}, map[string]string{}},
		{[]string{"foo=bar"}, map[string]string{"foo": "bar"}},
		{[]string{"foo=bar", "name=ava"}, map[string]string{"foo": "bar", "name": "ava"}},
		{[]string{"foo=bar", "name=ava", "foo=moo"},
			map[string]string{"foo": "moo", "name": "ava"}},
		{[]string{"foo=bar", "name=ava", "foo"}, nil},
	} {
		name := strings.Join(test.env, ";")
		t.Run(name, func(t *testing.T) {
			vars, err := Parse(test.env)
			if test.want != nil && err != nil {
				t.Fatalf("env.Parse(%v): %v", test.env, err)
			}
			if test.want == nil && err == nil {
				t.Fatalf("env.Parse(%v): unexpected succeess", test.env)
			}
			if test.want != nil {
				if diff := cmp.Diff(test.want, vars); diff != "" {
					t.Fatalf("env.Parse(%v) (-want +got):\n%s", test.env, diff)
				}
			}
		})
	}
}
