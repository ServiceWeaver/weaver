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
)

// Main is the name of the main component.
const Main = "github.com/ServiceWeaver/weaver/Main"

// CheckWeaveletArgs checks that WeaveletArgs is well-formed.
func CheckWeaveletArgs(w *protos.WeaveletArgs) error {
	if w == nil {
		return fmt.Errorf("WeaveletArgs: nil")
	}
	if w.App == "" {
		return fmt.Errorf("WeaveletArgs: missing app name")
	}
	if w.DeploymentId == "" {
		return fmt.Errorf("WeaveletArgs: missing deployment id")
	}
	if w.Id == "" {
		return fmt.Errorf("WeaveletArgs: missing weavelet id")
	}
	if w.ControlSocket == "" {
		return fmt.Errorf("WeaveletArgs: missing control socket")
	}
	return nil
}
