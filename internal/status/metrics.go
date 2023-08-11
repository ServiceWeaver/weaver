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

package status

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	dtool "github.com/ServiceWeaver/weaver/runtime/tool"
	"golang.org/x/exp/maps"
)

// MetricsCommand returns a "metrics" subcommand that pretty prints the metrics
// of all active applications registered with the provided registry. tool is
// the name of the command-line tool the returned subcommand runs as (e.g.,
// "weaver single").
func MetricsCommand(tool string, registry func(context.Context) (*Registry, error)) *dtool.Command {
	return &dtool.Command{
		Name:        "metrics",
		Description: "Show application metrics",
		Help: fmt.Sprintf(`Usage:
  %s metrics [metric regex]

Flags:
  -h, --help	Print this help message.

Description:
  "%s metrics" shows the latest value of every metric. You can filter
  metrics by providing a regular expression. Only the metrics with names that
  match the regular expression are shown. The command expects RE2 regular
  expressions, the same used by the built-in regexp module. See "go doc
  regexp/syntax" for details.

Examples:
  # Show all metrics
  %s metrics

  # Show metrics matching the regular expression "error"
  %s metrics error

  # Show metrics matching the regular expression "http"
  %s metrics http

  # Show metrics matching the regular expression "error" or "http"
  %s metrics 'error|http'`, tool, tool, tool, tool, tool, tool),
		Flags: flag.NewFlagSet("metrics", flag.ContinueOnError),
		Fn: func(ctx context.Context, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("too many arguments")
			}

			r, err := registry(ctx)
			if err != nil {
				return err
			}
			regs, err := r.List(ctx)
			if err != nil {
				return err
			}

			// Only show the metrics provided on the command line, or if there
			// are no command line arguments, show all metrics.
			matches := func(string) bool { return true }
			if len(args) == 1 {
				r, err := regexp.Compile(args[0])
				if err != nil {
					return fmt.Errorf("invalid regexp %q: %w", args[0], err)
				}
				matches = r.MatchString
			}

			var metrics []*protos.MetricSnapshot
			for _, reg := range regs {
				reply, err := NewClient(reg.Addr).Metrics(ctx)
				if err != nil {
					return err
				}
				for _, metric := range reply.Metrics {
					if matches(metric.Name) {
						metrics = append(metrics, metric)
					}
				}
			}
			formatMetrics(metrics)
			return nil
		},
	}
}

// formatMetrics pretty prints metrics to stdout.
func formatMetrics(metrics []*protos.MetricSnapshot) {
	// Group metrics by name.
	grouped := map[string][]*protos.MetricSnapshot{}
	for _, metric := range metrics {
		if strings.HasPrefix(metric.Name, "serviceweaver_system") {
			// Ignore Service Weaver internal metrics.
			continue
		}
		grouped[metric.Name] = append(grouped[metric.Name], metric)
	}

	// Sort metrics by name, sorting internal metrics after user metrics.
	names := maps.Keys(grouped)
	internal := func(name string) bool {
		return strings.HasPrefix(name, "serviceweaver")
	}
	sort.Slice(names, func(i, j int) bool {
		ni, nj := names[i], names[j]
		switch {
		case internal(ni) && !internal(nj):
			return false
		case !internal(ni) && internal(nj):
			return true
		case internal(ni) && internal(nj), !internal(ni) && !internal(nj):
			return ni < nj
		}
		return false
	})
	groups := make([][]*protos.MetricSnapshot, 0, len(names))
	for _, name := range names {
		groups = append(groups, grouped[name])
	}

	// Sort each metric by label.
	sortedValues := func(m map[string]string) []string {
		keys := maps.Keys(m)
		sort.Strings(keys)
		values := make([]string, len(keys))
		for i, key := range keys {
			values[i] = m[key]
		}
		return values
	}
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool {
			li, lj := group[i].Labels, group[j].Labels
			return slices.Compare(sortedValues(li), sortedValues(lj)) < 0
		})
	}

	// Pretty print the metrics.
	for _, group := range groups {
		name := group[0].Name
		typ := group[0].Typ
		help := group[0].Help
		keys := maps.Keys(group[0].Labels)
		sort.Strings(keys)

		title := []colors.Text{
			{
				{S: fmt.Sprintf("// %s", help), Color: colors.Color256(249)},
			},
			{
				{S: name, Color: colors.Color256(141), Bold: true},
				{S: ": "},
				{S: fmt.Sprint(typ), Color: colors.Color256(214)},
			},
		}
		t := colors.NewTabularizer(os.Stdout, title, colors.NoDim)

		// Write the header.
		var header []any
		for _, key := range keys {
			header = append(header, key)
		}
		if typ == protos.MetricType_HISTOGRAM {
			header = append(header, "Average")
		} else {
			header = append(header, "Value")
		}
		t.Row(header...)

		// Write the rows.
		dim := colors.Color256(245)
		for _, m := range group {
			var row []any
			for _, key := range keys {
				entry := colors.Atom{S: m.Labels[key]}

				// Abbreviate long labels.
				switch {
				case internal(name) && key == "component":
					entry.S = logging.ShortenComponent(entry.S)
				case key == "serviceweaver_node":
					entry.S = logging.Shorten(entry.S)
				case key == "serviceweaver_version":
					entry.Color = colors.ColorHash(entry.S)
					entry.S = logging.Shorten(entry.S)
					entry.Underline = true
				}

				// Dim zero-valued metrics.
				if m.Value == 0 {
					entry.Color = dim
				}

				row = append(row, entry)
			}

			entry := colors.Atom{S: fmt.Sprint(m.Value)}
			if typ == protos.MetricType_HISTOGRAM {
				count := uint64(0)
				for _, bucketCount := range m.Counts {
					count += bucketCount
				}
				if count != 0 {
					entry = colors.Atom{S: fmt.Sprint(m.Value / float64(count))}
				}
			}
			if m.Value == 0 {
				entry.Color = dim
			}
			row = append(row, entry)
			t.Row(row...)
		}
		t.Flush()
	}
}
