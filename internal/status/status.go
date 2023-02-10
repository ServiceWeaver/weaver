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
	"io"
	"sort"
	"strings"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	dtool "github.com/ServiceWeaver/weaver/runtime/tool"
)

// StatusCommand returns a "status" subcommand that pretty prints the status of
// all active applications registered with the provided registry. tool is the
// name of the command-line tool the returned subcommand runs as (e.g.,
// "weaver single").
func StatusCommand(tool string, registry func(context.Context) (*Registry, error)) *dtool.Command {
	return &dtool.Command{
		Name:        "status",
		Description: "Show the status of Service Weaver applications",
		Help: fmt.Sprintf(`Usage:
  %s status

Flags:
  -h, --help	Print this help message.`, tool),
		Flags: flag.NewFlagSet("status", flag.ContinueOnError),
		Fn: func(ctx context.Context, _ []string) error {
			r, err := registry(ctx)
			if err != nil {
				return err
			}
			regs, err := r.List(ctx)
			if err != nil {
				return err
			}
			var statuses []*Status
			for _, reg := range regs {
				status, err := NewClient(reg.Addr).Status(ctx)
				if err != nil {
					return err
				}
				statuses = append(statuses, status)
			}
			fmt.Print(format(statuses))
			return nil
		},
	}
}

// format pretty-prints the provided statuses.
func format(statuses []*Status) string {
	sort.Slice(statuses, func(i, j int) bool {
		// Sort by app name, breaking ties on age.
		x, y := statuses[i], statuses[j]
		if x.App != y.App {
			return x.App < y.App
		}
		return x.SubmissionTime.AsTime().Before(y.SubmissionTime.AsTime())
	})

	var b strings.Builder
	formatDeployments(&b, statuses)
	formatComponents(&b, statuses)
	formatListeners(&b, statuses)
	return b.String()
}

// formatId returns a pretty-printed prefix and suffix of the provided id. Both
// prefix and suffix are colored, and the prefix is underlined.
func formatId(id string) (prefix, suffix colors.Atom) {
	short := logging.Shorten(id)
	code := colors.ColorHash(id)
	prefix = colors.Atom{S: short, Color: code, Underline: true}
	suffix = colors.Atom{S: strings.TrimPrefix(id, short), Color: code}
	return prefix, suffix
}

// formatDeployments pretty-prints the set of deployments.
func formatDeployments(w io.Writer, statuses []*Status) {
	title := []colors.Text{{{S: "DEPLOYMENTS", Bold: true}}}
	t := colors.NewTabularizer(w, title, colors.PrefixDim)
	defer t.Flush()
	t.Row("APP", "DEPLOYMENT", "AGE")
	for _, status := range statuses {
		prefix, suffix := formatId(status.DeploymentId)
		age := time.Since(status.SubmissionTime.AsTime()).Truncate(time.Second)
		t.Row(status.App, colors.Text{prefix, suffix}, age)
	}
}

// formatDeployments pretty-prints the set of components.
func formatComponents(w io.Writer, statuses []*Status) {
	title := []colors.Text{{{S: "COMPONENTS", Bold: true}}}
	t := colors.NewTabularizer(w, title, colors.PrefixDim)
	defer t.Flush()
	t.Row("APP", "DEPLOYMENT", "COMPONENT", "REPLICA PIDS")
	for _, status := range statuses {
		sort.Slice(status.Components, func(i, j int) bool {
			return status.Components[i].Name < status.Components[j].Name
		})
		for _, component := range status.Components {
			prefix, _ := formatId(status.DeploymentId)
			c := logging.ShortenComponent(component.Name)
			sort.Slice(component.Pids, func(i, j int) bool {
				return component.Pids[i] < component.Pids[j]
			})
			pids := make([]string, len(component.Pids))
			for i, pid := range component.Pids {
				pids[i] = fmt.Sprint(pid)
			}
			t.Row(status.App, prefix, c, strings.Join(pids, ", "))
		}
	}
}

// formatDeployments pretty-prints the set of listeners.
func formatListeners(w io.Writer, statuses []*Status) {
	title := []colors.Text{{{S: "LISTENERS", Bold: true}}}
	t := colors.NewTabularizer(w, title, colors.PrefixDim)
	defer t.Flush()
	t.Row("APP", "DEPLOYMENT", "LISTENER", "ADDRESS")
	for _, status := range statuses {
		sort.Slice(status.Listeners, func(i, j int) bool {
			return status.Listeners[i].Name < status.Listeners[j].Name
		})
		for _, lis := range status.Listeners {
			prefix, _ := formatId(status.DeploymentId)
			t.Row(status.App, prefix, lis.Name, lis.Addr)
		}
	}
}
