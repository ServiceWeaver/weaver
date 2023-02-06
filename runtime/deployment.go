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
	"path/filepath"

	"github.com/google/uuid"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// CheckDeployment checks that a deployment is well-formed.
func CheckDeployment(d *protos.Deployment) error {
	if d == nil {
		return fmt.Errorf("nil deployment")
	}
	appName := d.App.Name
	if appName == "" {
		appName = filepath.Base(d.App.Binary)
	}
	if _, err := uuid.Parse(d.Id); err != nil {
		return fmt.Errorf("invalid deployment id for %s: %w", appName, err)
	}
	return nil
}

// DeploymentID returns the identifier that uniquely identifies a particular deployment.
func DeploymentID(d *protos.Deployment) uuid.UUID {
	id, err := uuid.Parse(d.Id)
	if err != nil {
		panic(fmt.Sprintf("bad UUID in internal proto: %v", err))
	}
	return id
}
