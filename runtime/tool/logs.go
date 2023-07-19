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

package tool

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/logging"
)

// LogSpec configures the command returned by LogsCmd.
type LogsSpec struct {
	Tool    string                                        // tool name, e.g., "weaver gke"
	Flags   *flag.FlagSet                                 // optional additional flags
	Rewrite func(logging.Query) (logging.Query, error)    // optional query preprocessing
	Source  func(context.Context) (logging.Source, error) // returns log source

	// Flags.
	follow bool
	format string
	system bool
}

// fullEntry is like runtime.LogEntry, but has all the fields present in the
// query language. Some entries, like full_version and full_node, are not
// present in a runtime.LogEntry; they are derived fields.
type fullEntry struct {
	App           string            `json:"app"`
	Version       string            `json:"version"`
	FullVersion   string            `json:"full_version"`
	Component     string            `json:"component"`
	FullComponent string            `json:"full_component"`
	Node          string            `json:"node"`
	FullNode      string            `json:"full_node"`
	Time          string            `json:"time"`
	Level         string            `json:"level"`
	File          string            `json:"file"`
	Line          int32             `json:"line"`
	Msg           string            `json:"msg"`
	Attrs         map[string]string `json:"attrs"`
}

// LogsCmd returns a command to query log entries.
func LogsCmd(spec *LogsSpec) *Command {
	// TODO(mwhittaker): Have documentation somewhere explaining what a node is.
	if spec.Flags == nil {
		spec.Flags = flag.NewFlagSet("logs", flag.ContinueOnError)
	}
	spec.Flags.BoolVar(&spec.follow, "follow", false, "Act like tail -f")
	spec.Flags.StringVar(&spec.format, "format", "pretty", "Output format (pretty or json)")
	spec.Flags.BoolVar(&spec.system, "system", false, "Show system internal logs")
	const help = `Usage:
  {{.Tool}} logs [--follow] [--format=<format>] [--system] [query]

Flags:
  -h, --help	Print this help message.
{{.Flags}}

Queries:
  Every log entry has the following fields:

      * app            : the application name
      * version        : the abbreviated application version
      * full_version   : the unabbreviated application version
      * component      : the abbreviated Service Weaver component name
      * full_component : the unabbreviated Service Weaver component name
      * node           : the abbreviated node name
      * full_node      : the unabbreviated node name
      * time           : the time the log entry was logged
      * level          : the level of the log (e.g., debug, info)
      * source         : the file:line from which the log entry was logged
      * msg            : the logged message
      * attrs          : the user provided attributes

  Queries are boolean expressions over these fields. For example, the query
  'app == "todo" && component == "Store"' matches every log entry with the app
  field set to "app" and the component field set to "Store". See the list of
  examples below for a demonstration of the types of queries you can write, and
  see the "Query Reference" section below for a complete description of the
  query language.

Examples:
  # Display all of the logs
  {{.Tool}} logs

  # Follow all of the logs (similar to tail -f).
  {{.Tool}} logs --follow

  # Display all of the logs for the "todo" app.
  {{.Tool}} logs 'app == "todo"'

  # Display all of the logs for version "cf575354" of the "todo" app.
  {{.Tool}} logs 'app == "todo" && version == "cf575354"'

  # Display all of the logs for component "store.Store" of version "cf575354" of
  # the "todo" app.
  {{.Tool}} logs 'app=="todo" && version=="cf575354" && component=="store.Store"'

  # Display all of the logs for node "3039a7d2" of version "cf575354" of the
  # "todo" app.
  {{.Tool}} logs 'app=="todo" && version=="cf575354" && node=="3039a7d2"'

  # Display all of the logs for the "todo" app that were logged on or after Jan
  # 1, 2022 (in the UTC+0 timezone).
  {{.Tool}} logs 'app=="todo" && time >= timestamp("2022-01-01T00:00:00Z")'

  # Note that timestamps must be in RFC 3339 format, with a "T" (e.g.,
  # "2022-07-22T06:58:01-07:00"). You can use the date command to generate
  # timestamps. For example, to get the timestamp of 3 hours ago, you can run
  # the following command.
  date --rfc-3339=s --date="3 hours ago" | tr ' ' 'T'

  # Display all of the debug logs for the "todo" app.
  {{.Tool}} logs 'app=="todo" && level=="debug"'

  # Display all of the info and error logs for the "todo" app.
  {{.Tool}} logs 'app=="todo" && (level=="info" || level=="error")'

  # Display all of the logs for the "todo" app, except for the debug logs.
  {{.Tool}} logs 'app=="todo" && level!="debug"'

  # Display all of the logs for the "todo" app in files called foo.go.
  {{.Tool}} logs 'app=="todo" && source.contains("foo.go")'

  # Display all of the logs that contain the string "error".
  {{.Tool}} logs 'msg.contains("error")'

  # Display all of the logs that match the regex "error: file .* already
  # closed". Regular expressions follow the RE2 syntax. See
  # https://github.com/google/re2/wiki/Syntax for details.
  {{.Tool}} logs 'msg.matches("error: file .* already closed")'

  # Display all of the logs that have an attribute "foo" with value "bar". Note that
  # when you write attrs["foo"], there is an implicit check that the "foo"
  # attribute exists.
  {{.Tool}} logs 'attrs["foo"] == "bar"'

  # Display all of the logs that have an attribute "foo" that isn't equal to "bar".
  # This query is logically the same as the query "foo" in attrs &&
  # attrs["foo"] != "bar".
  {{.Tool}} logs 'attrs["foo"] != "bar"'

  # Display all of the logs that either (a) don't have an attribute "foo" or (b) do
  # have an attribute "foo", but it's not equal "bar". This query is logically the
  # same as the query !("foo" in attrs && attrs["foo"] == "bar").
  {{.Tool}} logs '!(attrs["foo"] == "bar")'

  # Display all of the logs that have a "foo" attribute.
  {{.Tool}} logs '"foo" in attrs'

  # Display all of the logs that don't have a "foo" attribute.
  {{.Tool}} logs '!("foo" in attrs)'

  # Display all of the logs in JSON format. This is useful if you want to
  # perform some sort of post-processing on the logs.
  {{.Tool}} logs --format=json

  # Display all of the logs, including internal system logs that are hidden by
  # default.
  {{.Tool}} logs --system

  # Display all of the logs, but without color.
  NO_COLOR= {{.Tool}} logs

Query Reference:
  Queries are written using a subset of the CEL language [1]. Thus, every
  syntactically valid query is also a syntactically valid CEL program.
  Specifically, a query is a CEL program over the following fields:

      * app: string
      * version: string
      * full_version: string
      * component: string
      * full_component: string
      * node: string
      * full_node: string
      * time: timestamp
      * level: string
      * source: string
      * msg: string
      * attrs: map[string]string

  A query is restricted to:

      * boolean algebra (!, &&, ||),
      * equalities and inequalities (==, !=, <, <=, >, >=),
      * the string operations "contains" and "matches",
      * map indexing (attrs["foo"]), and
      * constant strings, timestamps, and ints.

  Queries have the same semantics as CEL programs except for one small
  exception. An attribute expression like attrs["foo"] has an implicit
  membership test "foo" in attrs.

  [1]: https://opensource.google/projects/cel`
	var b strings.Builder
	t := template.Must(template.New(spec.Tool).Parse(help))
	content := struct{ Tool, Flags string }{spec.Tool, FlagsHelp(spec.Flags)}
	if err := t.Execute(&b, content); err != nil {
		panic(err)
	}

	return &Command{
		Name:        "logs",
		Flags:       spec.Flags,
		Description: "Cat or follow Service Weaver logs",
		Help:        b.String(),
		Fn:          spec.logFn,
	}
}

func (s *LogsSpec) logFn(ctx context.Context, args []string) error {
	// Parse command line arguments.
	if len(args) > 1 {
		return fmt.Errorf("too many arguments")
	}
	var query string
	if len(args) == 0 {
		// If no query is provided, we want to show all logs. To do that, we
		// use the following query, which evaluates to true for every log
		// entry.
		query = `app.contains("")`
	} else {
		query = args[0]
	}
	if s.format != "pretty" && s.format != "json" {
		return fmt.Errorf("invalid format %q; must be %q or %q", s.format, "pretty", "json")
	}

	// Rewrite the query, if needed.
	if s.Rewrite != nil {
		var err error
		query, err = s.Rewrite(query)
		if err != nil {
			return err
		}
	}

	// Show or hide system logs.
	if !s.system {
		query += ` && !("serviceweaver/system" in attrs)`
	}

	// Construct the reader.
	source, err := s.Source(ctx)
	if err != nil {
		return err
	}
	r, err := source.Query(ctx, query, s.follow)
	if err != nil {
		return err
	}

	// Cat or follow the logs.
	pp := logging.NewPrettyPrinter(colors.Enabled())
	for {
		entry, err := r.Read(ctx)
		if errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
		switch s.format {
		case "pretty":
			fmt.Println(pp.Format(entry))
		case "json":
			attrs := map[string]string{}
			for i := 0; i < len(entry.Attrs); i += 2 {
				key := entry.Attrs[i]
				value := ""
				if i+1 < len(entry.Attrs) {
					value = entry.Attrs[i+1]
				}
				attrs[key] = value
			}
			bytes, err := json.MarshalIndent(fullEntry{
				App:           entry.App,
				Version:       logging.Shorten(entry.Version),
				FullVersion:   entry.Version,
				Component:     logging.ShortenComponent(entry.Component),
				FullComponent: entry.Component,
				Node:          logging.Shorten(entry.Node),
				FullNode:      entry.Node,
				Time:          time.UnixMicro(entry.TimeMicros).Format(time.RFC3339Nano),
				Level:         entry.Level,
				File:          entry.File,
				Line:          entry.Line,
				Msg:           entry.Msg,
				Attrs:         attrs,
			}, "", "    ")
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
		default:
			panic(fmt.Sprintf("unexpected format %q", s.format))
		}
	}
}
