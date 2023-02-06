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

package babysitter

import (
	"context"

	"github.com/ServiceWeaver/weaver/runtime/envelope"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/tool"
)

// runProfiling runs a profiling request on a set of processes.
func runProfiling(ctx context.Context, req *protos.RunProfiling,
	processes map[string][]*envelope.Envelope) (*protos.Profile, error) {
	// Collect together the groups we want to profile.
	groups := make([][]func() (*protos.Profile, error), 0, len(processes))
	for _, envelopes := range processes {
		group := make([]func() (*protos.Profile, error), 0, len(envelopes))
		for _, e := range envelopes {
			group = append(group, func() (*protos.Profile, error) {
				return e.RunProfiling(ctx, req)
			})
		}
		groups = append(groups, group)
	}
	return tool.ProfileGroups(groups)
}
