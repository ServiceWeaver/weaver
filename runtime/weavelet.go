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

	"github.com/google/uuid"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// CheckWeavelet checks that weavelet information is well-formed.
func CheckWeavelet(d *protos.Weavelet) error {
	if d == nil {
		return fmt.Errorf("nil weavelet")
	}
	if d.Process == "" {
		return fmt.Errorf("empty process name in weavelet")
	}
	if _, err := uuid.Parse(d.Id); err != nil {
		return fmt.Errorf("invalid weavelet id for %s: %w", d.Process, err)
	}
	if err := CheckDeployment(d.Dep); err != nil {
		return err
	}
	if err := checkColocationGroup(d.Group); err != nil {
		return err
	}
	return nil
}

// checkColocationGroup checks that a colocation group is well-formed.
func checkColocationGroup(g *protos.ColocationGroup) error {
	if g == nil {
		return fmt.Errorf("nil colocation group")
	}
	if g.Name == "" {
		return fmt.Errorf("empty colocation group name")
	}
	return nil
}
