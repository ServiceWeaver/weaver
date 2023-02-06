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
	"bytes"
	"fmt"
	"sync"

	"github.com/google/pprof/profile"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// ProfileGroups returns the approximate sum of the profiles of all members of all groups.
// If a group has more than one member, only a subset of the group may be profiled and the
// result scaled up assuming non-profiled members of the group have similar profiles.
// Each group member is represented by a function that profiles that group member.
func ProfileGroups(groups [][]func() (*protos.Profile, error)) (*protos.Profile, error) {
	type groupState struct {
		profile *profile.Profile
		errs    []string
	}
	profiles := make([]groupState, len(groups))

	// Profile each group in parallel.
	var wait sync.WaitGroup
	for i, group := range groups {
		if len(group) == 0 {
			// Skip empty groups.
			continue
		}
		i := i
		group := group
		wait.Add(1)
		go func() {
			defer wait.Done()

			// Try each member until we get a profile.
			var lastErr error
			var p *profile.Profile
			for _, fn := range group {
				reply, err := fn()
				if err != nil {
					lastErr = err
					continue
				}
				if len(reply.Errors) > 0 {
					lastErr = fmt.Errorf("%v", reply.Errors)
				}
				if len(reply.Data) == 0 {
					p = nil
					break
				}

				// Parse the profile data and scale it based on the number of replicas
				// for the given process
				prof, err := profile.ParseData(reply.Data)
				if err != nil {
					lastErr = err
					continue
				}
				if len(group) > 1 {
					prof.Scale(float64(len(group)))
				}
				p = prof
				break
			}
			if lastErr != nil {
				profiles[i].errs = append(profiles[i].errs, lastErr.Error())
			}
			profiles[i].profile = p
		}()
	}
	wait.Wait()

	// Fill the final merged profile.
	var profs []*profile.Profile
	var errs []string
	for _, p := range profiles {
		errs = append(errs, p.errs...)
		if p.profile != nil {
			profs = append(profs, p.profile)
		}
	}
	if len(profs) == 0 && len(errs) > 0 {
		// No profile data but have errors: that's an error.
		return nil, fmt.Errorf("error collecting profile: %v", errs)
	}

	var buf bytes.Buffer
	if len(profs) > 0 {
		merged := profs[0]
		if len(profs) > 1 {
			var err error
			if merged, err = profile.Merge(profs); err != nil {
				return nil, err
			}
		}
		if err := merged.Write(&buf); err != nil {
			return nil, err
		}
	}
	return &protos.Profile{Data: buf.Bytes(), Errors: errs}, nil
}
