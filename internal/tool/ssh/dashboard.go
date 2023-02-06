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
	"fmt"

	"github.com/ServiceWeaver/weaver/internal/status"
	"github.com/ServiceWeaver/weaver/internal/tool/ssh/impl"
	"github.com/ServiceWeaver/weaver/runtime/logging"
)

var dashboardSpec = &status.DashboardSpec{
	Tool:     "weaver ssh",
	Registry: impl.DefaultRegistry,
	Commands: func(deploymentId string) []status.Command {
		return []status.Command{
			{Label: "cat logs", Command: fmt.Sprintf("weaver ssh logs 'version==%q'", logging.Shorten(deploymentId))},
			{Label: "follow logs", Command: fmt.Sprintf("weaver ssh logs --follow 'version==%q'", logging.Shorten(deploymentId))},
		}
	},
}
