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

package multi

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ServiceWeaver/weaver/internal/must"
	"github.com/ServiceWeaver/weaver/internal/status"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/tool"
)

var (
	// The directories and files where "weaver multi" stores data.
	logDir       = filepath.Join(runtime.LogsDir(), "multi")
	dataDir      = filepath.Join(must.Must(runtime.DataDir()), "multi")
	registryDir  = filepath.Join(dataDir, "registry")
	perfettoFile = filepath.Join(dataDir, "perfetto.db")

	dashboardSpec = &status.DashboardSpec{
		Tool:         "weaver multi",
		PerfettoFile: perfettoFile,
		Registry:     defaultRegistry,
		Commands: func(deploymentId string) []status.Command {
			return []status.Command{
				{Label: "status", Command: "weaver multi status"},
				{Label: "cat logs", Command: fmt.Sprintf("weaver multi logs 'version==%q'", logging.Shorten(deploymentId))},
				{Label: "follow logs", Command: fmt.Sprintf("weaver multi logs --follow 'version==%q'", logging.Shorten(deploymentId))},
				{Label: "profile", Command: fmt.Sprintf("weaver multi profile --duration=30s %s", deploymentId)},
			}
		},
	}

	purgeSpec = &tool.PurgeSpec{
		Tool:  "weaver multi",
		Kill:  "weaver multi (dashboard|deploy|logs|profile)",
		Paths: []string{logDir, dataDir},
	}

	Commands = map[string]*tool.Command{
		"deploy": &deployCmd,
		"logs": tool.LogsCmd(&tool.LogsSpec{
			Tool: "weaver multi",
			Source: func(context.Context) (logging.Source, error) {
				return logging.FileSource(logDir), nil
			},
		}),
		"dashboard": status.DashboardCommand(dashboardSpec),
		"status":    status.StatusCommand("weaver multi", defaultRegistry),
		"metrics":   status.MetricsCommand("weaver multi", defaultRegistry),
		"profile":   status.ProfileCommand("weaver multi", defaultRegistry),
		"purge":     tool.PurgeCmd(purgeSpec),
		"version":   tool.VersionCmd("weaver multi"),
	}
)
