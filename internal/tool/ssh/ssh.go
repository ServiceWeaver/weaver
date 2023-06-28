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

package ssh

import (
	"path/filepath"

	"github.com/ServiceWeaver/weaver/internal/must"
	"github.com/ServiceWeaver/weaver/internal/status"
	"github.com/ServiceWeaver/weaver/internal/tool/ssh/impl"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/tool"
)

var (
	purgeSpec = &tool.PurgeSpec{
		Tool: "weaver ssh",
		Kill: "weaver ssh (dashboard|deploy|logs|profile)",
		Paths: []string{filepath.Join(runtime.LogsDir(), "ssh"),
			filepath.Join(must.Must(runtime.DataDir()), "ssh")},
	}

	Commands = map[string]*tool.Command{
		"deploy":    &deployCmd,
		"logs":      tool.LogsCmd(&logsSpec),
		"dashboard": status.DashboardCommand(dashboardSpec),
		"metrics":   status.MetricsCommand("weaver ssh", impl.DefaultRegistry),
		"profile":   status.ProfileCommand("weaver ssh", impl.DefaultRegistry),
		"purge":     tool.PurgeCmd(purgeSpec),
		"status":    status.StatusCommand("weaver ssh", impl.DefaultRegistry),
		"version":   tool.VersionCmd("weaver ssh"),

		// Hidden commands.
		"babysitter": &babysitterCmd,
	}
)
