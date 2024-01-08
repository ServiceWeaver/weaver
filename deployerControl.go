// Copyright 2024 Google LLC
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
	"context"
	"fmt"
	"os"

	"github.com/ServiceWeaver/weaver/internal/control"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// deployerControl is a component hosted in every deployer. Weavelets make calls to this
// component to interact with the deployer.
type deployerControl control.DeployerControl

// localDeployerControl is the implementation of deployerControl for local execution. It is
// overridden by remote deployers.
type localDeployerControl struct {
	Implements[deployerControl]
	pp *logging.PrettyPrinter
}

var _ deployerControl = &localDeployerControl{}

// Init initializes the local deployerControl component.
func (local *localDeployerControl) Init(ctx context.Context) error {
	local.pp = logging.NewPrettyPrinter(colors.Enabled())
	return nil
}

// LogBatch logs a list of entries.
func (local *localDeployerControl) LogBatch(ctx context.Context, batch *protos.LogEntryBatch) error {
	for _, entry := range batch.Entries {
		fmt.Fprintln(os.Stderr, local.pp.Format(entry))
	}
	return nil
}
