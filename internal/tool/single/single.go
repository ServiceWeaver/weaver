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

package single

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ServiceWeaver/weaver/internal/must"
	"github.com/ServiceWeaver/weaver/internal/status"
	itool "github.com/ServiceWeaver/weaver/internal/tool"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/tool"
)

var (
	// The directories and files where the single process deployer stores data.
	dataDir      = filepath.Join(must.Must(runtime.DataDir()), "single")
	RegistryDir  = filepath.Join(dataDir, "registry")
	PerfettoFile = filepath.Join(dataDir, "traces.DB")

	dashboardSpec = &status.DashboardSpec{
		Tool:         "weaver single",
		PerfettoFile: PerfettoFile,
		Registry:     defaultRegistry,
		Commands: func(deploymentId string) []status.Command {
			return []status.Command{
				{Label: "status", Command: "weaver single status"},
				{Label: "profile", Command: fmt.Sprintf("weaver single profile --duration=30s %s", deploymentId)},
			}
		},
	}
	purgeSpec = &tool.PurgeSpec{
		Tool:  "weaver single",
		Kill:  "weaver single (dashboard|profile)",
		Paths: []string{dataDir},
	}

	Commands = map[string]*tool.Command{
		"deploy":    &deployCmd,
		"status":    status.StatusCommand("weaver single", defaultRegistry),
		"dashboard": status.DashboardCommand(dashboardSpec),
		"metrics":   status.MetricsCommand("weaver single", defaultRegistry),
		"profile":   status.ProfileCommand("weaver single", defaultRegistry),
		"purge":     tool.PurgeCmd(purgeSpec),
		"version":   itool.VersionCmd("weaver single"),
	}
)

func defaultRegistry(ctx context.Context) (*status.Registry, error) {
	return status.NewRegistry(ctx, RegistryDir)
}
