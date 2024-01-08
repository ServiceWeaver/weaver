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

package control

import (
	"context"

	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// DeployerControl is the interface for the weaver.deployerControl component. It is
// present in its own package so other packages do not need to copy the interface
// definition.
//
// Arguments and results are protobufs to allow deployers to evolve independently
// of application binaries.
type DeployerControl interface {
	LogBatch(context.Context, *protos.LogEntryBatch) error
}
