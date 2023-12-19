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

package weaver

import (
	"bytes"
	"context"
	"fmt"
	"runtime/pprof"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// getProfile collects a profile of this process.
func getProfile(ctx context.Context, req *protos.GetProfileRequest) ([]byte, error) {
	var buf bytes.Buffer
	switch req.ProfileType {
	case protos.ProfileType_Heap:
		if err := pprof.WriteHeapProfile(&buf); err != nil {
			return nil, err
		}
	case protos.ProfileType_CPU:
		if req.CpuDurationNs == 0 {
			return nil, fmt.Errorf("invalid zero duration for the CPU profile collection")
		}
		dur := time.Duration(req.CpuDurationNs) * time.Nanosecond
		if err := pprof.StartCPUProfile(&buf); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			pprof.StopCPUProfile()
			return nil, ctx.Err()
		case <-time.After(dur):
			// All done
		}
		pprof.StopCPUProfile()
	default:
		return nil, fmt.Errorf("unspecified profile collection type")
	}
	return buf.Bytes(), nil
}
