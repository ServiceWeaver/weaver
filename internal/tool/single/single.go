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

	"github.com/ServiceWeaver/weaver/internal/files"
	"github.com/ServiceWeaver/weaver/internal/status"
	"github.com/ServiceWeaver/weaver/runtime/tool"
)

var (
	dashboardSpec = &status.DashboardSpec{
		Tool:     "weaver single",
		Registry: defaultRegistry,
		Commands: func(deploymentId string) []status.Command {
			return []status.Command{
				{Label: "status", Command: "weaver single status"},
				{Label: "profile", Command: fmt.Sprintf("weaver single profile --duration=30s %s", deploymentId)},
			}
		},
	}

	Commands = map[string]*tool.Command{
		"status":    status.StatusCommand("weaver single", defaultRegistry),
		"dashboard": status.DashboardCommand(dashboardSpec),
		"metrics":   status.MetricsCommand("weaver single", defaultRegistry),
		"profile":   status.ProfileCommand("weaver single", defaultRegistry),
	}
)

func defaultRegistry(ctx context.Context) (*status.Registry, error) {
	dir, err := files.DefaultDataDir()
	if err != nil {
		return nil, err
	}
	return status.NewRegistry(ctx, filepath.Join(dir, "single_registry"))
}
