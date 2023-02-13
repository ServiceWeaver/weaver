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

package logging

import (
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/cel-go/parser"
)

func TestValidQueries(t *testing.T) {
	for _, query := range []string{
		`app == "todo"`,
		`app != "todo"`,
		`app.contains("todo")`,
		`app.matches("todo")`,
		`attrs["name"] == "foo"`,
		`attrs["name"].contains("foo")`,
		`"foo" in attrs`,
		`time < timestamp("1972-01-01T10:00:20.021-05:00")`,
		`app == "todo" && version == "v1"`,
		`app == "todo" && full_version == "v1"`,
		`app == "todo" || app == "collatz"`,
		`!(app == "todo")`,
		`app == "todo" &&
		    (version == "v1" || version == "v2") &&
		    !(node == "123") &&
		    time < timestamp("1972-01-01T10:00:20.021-05:00") &&
		    level.contains("e") &&
		    (source.matches("^main.go") || source.matches("^foo.go")) &&
		    msg.matches("error") &&
			"foo" in attrs &&
			attrs["foo"] == "bar"`,
	} {
		t.Run(query, func(t *testing.T) {
			env, ast, err := parse(query)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := compile(env, ast); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInvalidQueries(t *testing.T) {
	for _, query := range []string{
		// Unsupported operations and calls.
		`app == "todo" ? app == "todo" : app == "todo"`, // conditional
		`app == ["todo"][0]`,                            // index
		`app == {"key": "todo"}["key"]`,                 // index
		`true in [true]`,                                // in
		`"key" in {"key": true}`,                        // in
		`bool(true)`,                                    // bool
		`string("foo")`,                                 // string
		`bytes(b"foo")`,                                 // bytes
		`int(1)`,                                        // int
		`uint(1)`,                                       // uint
		`double(1.0)`,                                   // double
		`type(1)`,                                       // type
		`"foo".startsWith("foo")`,                       // startsWith
		`"foo".endsWith("foo")`,                         // endsWith

		// Bad LHS.
		`"todo" == app`,
		`"todo" == attrs["foo"]`,
		`source in attrs`,
		`attrs["foo"] in attrs`,
		`timestamp("1972-01-01T10:00:20.021-05:00") > time`,

		// Bad RHS.
		`source == source`,
		`attrs["foo"] == attrs["foo"]`,

		// Unsupported root operations.
		`true`,   // bool
		`!false`, // not

		// Wrong types.
		`"foo"`,        // string
		`b"foo"`,       // bytes
		`1.0`,          // double
		`[true]`,       // list
		`{true: true}`, // map
	} {
		t.Run(query, func(t *testing.T) {
			// The query should be valid CEL, but not a valid query.
			env, err := env()
			if err != nil {
				t.Fatal(err)
			}
			if _, issues := env.Compile(query); issues != nil && issues.Err() != nil {
				t.Fatal(issues.Err())
			}

			if _, _, err := parse(query); err == nil {
				t.Fatalf("compile(%s): unexpected success", query)
			}
		})
	}
}

func TestRewrite(t *testing.T) {
	for _, test := range []struct {
		query Query
		want  string
	}{
		{`app == "todo"`, `app == "todo"`},
		{`app != "todo"`, `app != "todo"`},
		{`app.contains("todo")`, `app.contains("todo")`},
		{`app.matches("todo")`, `app.matches("todo")`},
		{`attrs["name"] == "foo"`, `"name" in attrs && attrs["name"] == "foo"`},
		{`attrs["name"].contains("foo")`, `"name" in attrs && attrs["name"].contains("foo")`},
		{`"foo" in attrs`, `"foo" in attrs`},
		{`time < timestamp("1972-01-01T10:00:20.021-05:00")`, `time < timestamp("1972-01-01T10:00:20.021-05:00")`},
		{`app == "todo" && version == "v1"`, `app == "todo" && version == "v1"`},
		{`app == "todo" && full_version == "v1"`, `app == "todo" && full_version == "v1"`},
		{`app == "todo" || app == "collatz"`, `app == "todo" || app == "collatz"`},
		{`app == "todo" || attrs["foo"] == "bar"`, `app == "todo" || "foo" in attrs && attrs["foo"] == "bar"`},
		{`!(app == "todo")`, `!(app == "todo")`},
		{`!(attrs["foo"] == "bar")`, `!("foo" in attrs && attrs["foo"] == "bar")`},
	} {
		t.Run(test.query, func(t *testing.T) {
			env, ast, err := parse(test.query)
			if err != nil {
				t.Fatal(err)
			}
			e, err := rewrite(ast.Expr())
			if err != nil {
				t.Fatal(err)
			}
			q, err := format(e)
			if err != nil {
				t.Fatal(err)
			}
			ast, issues := env.Compile(q)
			if issues != nil && issues.Err() != nil {
				t.Fatal(issues.Err())
			}
			got, err := parser.Unparse(ast.Expr(), ast.SourceInfo())
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("bad rewrite: got %v, want %v", got, test.want)
			}
		})
	}
}

// at returns a time `seconds` seconds after 2000-01-01.
// The return value is microseconds since the epich.
func at(seconds int) int64 {
	t := time.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC)
	return t.Add(time.Duration(seconds) * time.Second).UnixMicro()
}

func TestQueryMatches(t *testing.T) {
	const simple = `app=="a" && component=="o"`
	const advanced = `
		app == "a" &&
		(version == "v1" || full_version == "v2") &&
		component != "o" &&
		node != "123" &&
		time >= timestamp("2000-01-01T00:00:10.000-00:00") &&
		time <= timestamp("2000-01-01T00:00:20.000-00:00") &&
		(level == "debug" || level == "info" || level == "warn") &&
		(!source.matches("^foo.go") && source.matches("^bar.go")) &&
		msg.contains("a")
	`

	for _, test := range []struct {
		name  string
		query Query
		entry *protos.LogEntry
		want  bool
	}{
		// Simple matches.
		{"Simple/Basic", simple, &protos.LogEntry{App: "a", Component: "o"}, true},
		{"Simple/Version", simple, &protos.LogEntry{App: "a", Version: "v1", Component: "o"}, true},
		{"Simple/Node", simple, &protos.LogEntry{App: "a", Component: "o", Node: "123"}, true},
		{"Simple/Time", simple, &protos.LogEntry{App: "a", Component: "o", TimeMicros: at(10)}, true},
		{"Simple/Level", simple, &protos.LogEntry{App: "a", Component: "o", Level: "warn"}, true},
		{"Simple/Source", simple, &protos.LogEntry{App: "a", Component: "o", File: "foo.go", Line: 10}, true},
		{"Simple/Msg", simple, &protos.LogEntry{App: "a", Component: "o", Msg: "foo"}, true},

		// Simple doesn't match.
		{"Simple/nil", simple, nil, false},
		{"Simple/BadApp", simple, &protos.LogEntry{App: "aa", Component: "o"}, false},
		{"Simple/BadComponent", simple, &protos.LogEntry{App: "a", Component: "oo"}, false},

		// Advanced matches.
		{"Advanced/Matches", advanced, &protos.LogEntry{
			App:        "a",
			Version:    "v1",
			Component:  "not o",
			Node:       "not 123",
			TimeMicros: at(15),
			Level:      "info",
			File:       "bar.go",
			Line:       5,
			Msg:        "banana",
		}, true},

		// Advanced doesn't match.
		{"Advanced/nil", advanced, nil, false},
		{"Advanced/BadVersion", advanced, &protos.LogEntry{
			App:        "a",
			Version:    "v3",
			Component:  "not o",
			Node:       "not 123",
			TimeMicros: at(15),
			Level:      "info",
			File:       "bar.go",
			Line:       5,
			Msg:        "banana",
		}, false},
		{"Advanced/BadComponent", advanced, &protos.LogEntry{
			App:        "a",
			Version:    "v2",
			Component:  "o",
			Node:       "not 123",
			TimeMicros: at(15),
			Level:      "info",
			File:       "bar.go",
			Line:       5,
			Msg:        "banana",
		}, false},
		{"Advanced/BadNode", advanced, &protos.LogEntry{
			App:        "a",
			Version:    "v2",
			Component:  "not o",
			Node:       "123",
			TimeMicros: at(15),
			Level:      "info",
			File:       "bar.go",
			Line:       5,
			Msg:        "banana",
		}, false},
		{"Advanced/BadTime", advanced, &protos.LogEntry{
			App:        "a",
			Version:    "v1",
			Component:  "not o",
			Node:       "not 123",
			TimeMicros: at(5),
			Level:      "info",
			File:       "bar.go",
			Line:       5,
			Msg:        "banana",
		}, false},

		{"Advanced/BadLevel", advanced, &protos.LogEntry{
			App:        "a",
			Version:    "v1",
			Component:  "not o",
			Node:       "not 123",
			TimeMicros: at(15),
			Level:      "error",
			File:       "bar.go",
			Line:       5,
			Msg:        "banana",
		}, false},
		{"Advanced/BadSource", advanced, &protos.LogEntry{
			App:        "a",
			Version:    "v1",
			Component:  "not o",
			Node:       "not 123",
			TimeMicros: at(15),
			Level:      "info",
			File:       "foo.go",
			Line:       15,
			Msg:        "banana",
		}, false},
		{"Advanced/BadMsg", advanced, &protos.LogEntry{
			App:        "a",
			Version:    "v1",
			Component:  "not o",
			Node:       "not 123",
			TimeMicros: at(15),
			Level:      "info",
			File:       "bar.go",
			Line:       5,
			Msg:        "bnn",
		}, false},

		// Attr matches.
		{"Attrs/Equals", `attrs["foo"]=="bar"`, &protos.LogEntry{Attrs: []string{"foo", "bar"}}, true},
		{"Attrs/Contains", `attrs["foo"].contains("b")`, &protos.LogEntry{Attrs: []string{"foo", "bar"}}, true},
		{"Attrs/Path", `attrs["foo.bar"]=="baz"`, &protos.LogEntry{Attrs: []string{"foo.bar", "baz"}}, true},
		{"Attrs/Nil", `attrs["foo"]=="bar"`, &protos.LogEntry{}, false},
		{"Attrs/Empty", `attrs["foo"]=="bar"`, &protos.LogEntry{Attrs: []string{}}, false},
		{"Attrs/NoVal", `attrs["foo"]=="bar"`, &protos.LogEntry{Attrs: []string{"foo"}}, false},
		{"Attrs/Missing", `attrs["foo"]=="bar"`, &protos.LogEntry{Attrs: []string{"bar", "foo"}}, false},
		{"Attrs/IndexMissing", `attrs["foo"]=="bar"`, &protos.LogEntry{Attrs: []string{"bar", "foo"}}, false},
		{"Attrs/NotEqualFalse", `attrs["foo"]!="bar"`, &protos.LogEntry{Attrs: []string{"foo", "bar"}}, false},
		{"Attrs/NotEqualTrue", `attrs["foo"]!="bar"`, &protos.LogEntry{Attrs: []string{"foo", "not bar"}}, true},
		{"Attrs/NotEqualMissing", `attrs["foo"]!="bar"`, &protos.LogEntry{Attrs: []string{}}, false},
		{"Attrs/NotOfEqualFalse", `!(attrs["foo"]=="bar")`, &protos.LogEntry{Attrs: []string{"foo", "bar"}}, false},
		{"Attrs/NotOfEqualTrue", `!(attrs["foo"]=="bar")`, &protos.LogEntry{Attrs: []string{"foo", "not bar"}}, true},
		{"Attrs/NotOfEqualMissing", `!(attrs["foo"]=="bar")`, &protos.LogEntry{Attrs: []string{}}, true},
		{"Attrs/In", `"foo" in attrs`, &protos.LogEntry{Attrs: []string{"foo", "bar"}}, true},
		{"Attrs/NotIn", `"foo" in attrs`, &protos.LogEntry{Attrs: []string{}}, false},

		// version vs full_version.
		{"Version/1", `version=="1"`, &protos.LogEntry{Version: "1"}, true},
		{"Version/1-8", `version=="12345678"`, &protos.LogEntry{Version: "12345678"}, true},
		{"Version/1-8/1-f", `version=="12345678"`, &protos.LogEntry{Version: "123456789abcdef"}, true},
		{"Version/1-f/1-f", `version=="123456789abcdef"`, &protos.LogEntry{Version: "123456789abcdef"}, false},
		{"FullVersion/1", `full_version=="1"`, &protos.LogEntry{Version: "1"}, true},
		{"FullVersion/1-8", `full_version=="12345678"`, &protos.LogEntry{Version: "12345678"}, true},
		{"FullVersion/1-8/1-f", `full_version=="12345678"`, &protos.LogEntry{Version: "123456789abcdef"}, false},
		{"FullVersion/1-f/1-f", `full_version=="123456789abcdef"`, &protos.LogEntry{Version: "123456789abcdef"}, true},

		// node vs full_node.
		{"Node/1", `node=="1"`, &protos.LogEntry{Node: "1"}, true},
		{"Node/1-8", `node=="12345678"`, &protos.LogEntry{Node: "12345678"}, true},
		{"Node/1-8/1-f", `node=="12345678"`, &protos.LogEntry{Node: "123456789abcdef"}, true},
		{"Node/1-f/1-f", `node=="123456789abcdef"`, &protos.LogEntry{Node: "123456789abcdef"}, false},
		{"FullNode/1", `full_node=="1"`, &protos.LogEntry{Node: "1"}, true},
		{"FullNode/1-8", `full_node=="12345678"`, &protos.LogEntry{Node: "12345678"}, true},
		{"FullNode/1-8/1-f", `full_node=="12345678"`, &protos.LogEntry{Node: "123456789abcdef"}, false},
		{"FullNode/1-f/1-f", `full_node=="123456789abcdef"`, &protos.LogEntry{Node: "123456789abcdef"}, true},

		// component vs full_component.
		{"Component/NoPkg", `component=="Foo"`, &protos.LogEntry{Component: "Foo"}, true},
		{"Component/Pkg", `component=="c.Foo"`, &protos.LogEntry{Component: "a/b/c/Foo"}, true},
		{"Component/Full", `component=="a/b/c/Foo"`, &protos.LogEntry{Component: "a/b/c/Foo"}, false},
		{"FullComponent/NoPkg", `full_component=="Foo"`, &protos.LogEntry{Component: "Foo"}, true},
		{"FullComponent/Pkg", `full_component=="a/b/c/Foo"`, &protos.LogEntry{Component: "a/b/c/Foo"}, true},
		{"FullComponent/Suffix", `full_component=="c/Foo"`, &protos.LogEntry{Component: "a/b/c/Foo"}, false},
		{"FullComponent/Short", `full_component=="c.Foo"`, &protos.LogEntry{Component: "a/b/c/Foo"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			env, ast, err := parse(test.query)
			if err != nil {
				t.Fatalf("parse(%v): %v", test.query, err)
			}
			prog, err := compile(env, ast)
			if err != nil {
				t.Fatalf("compile(%v): %v", test.query, err)
			}
			got, err := matches(prog, test.entry)
			if err != nil {
				t.Fatalf("matches(%v, %v): %v", test.query, test.entry, err)
			}
			if got != test.want {
				t.Errorf("matches(%v, %v): got %t, want %t", test.query, test.entry, got, test.want)
			}
		})
	}
}
