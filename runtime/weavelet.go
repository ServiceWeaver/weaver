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

package runtime

import (
	"fmt"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
)

// CheckEnvelopeInfo checks that EnvelopeInfo is well-formed.
func CheckEnvelopeInfo(w *protos.EnvelopeInfo) error {
	if w == nil {
		return fmt.Errorf("EnvelopeInfo: nil")
	}
	if w.App == "" {
		return fmt.Errorf("EnvelopeInfo: missing app name")
	}
	if _, err := uuid.Parse(w.DeploymentId); err != nil {
		return fmt.Errorf("EnvelopeInfo: invalid deployment id: %w", err)
	}
	if _, err := uuid.Parse(w.Id); err != nil {
		return fmt.Errorf("EnvelopeInfo: invalid weavelet id: %w", err)
	}
	if w.SingleProcess && !w.SingleMachine {
		return fmt.Errorf("EnvelopeInfo: single process but not single machine")
	}
	return nil
}
