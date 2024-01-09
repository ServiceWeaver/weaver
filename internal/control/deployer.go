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

// DeployerPath is the path used for the deployer control component.
// It points to an internal type in a different package.
const DeployerPath = "github.com/ServiceWeaver/weaver/deployerControl"

// DeployerControl is the interface for the weaver.deployerControl component. It is
// present in its own package so other packages do not need to copy the interface
// definition.
//
// Arguments and results are protobufs to allow deployers to evolve independently
// of application binaries.
type DeployerControl interface {
	// LogBatch logs the supplied batch of log entries.
	LogBatch(context.Context, *protos.LogEntryBatch) error

	// ActivateComponent ensures that the provided component is running
	// somewhere. A call to ActivateComponent also implicitly signals that a
	// weavelet is interested in receiving routing info for the component.
	ActivateComponent(context.Context, *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error)

	// GetListenerAddress returns the address the weavelet should listen on for
	// a particular listener.
	GetListenerAddress(context.Context, *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error)

	// ExportListener exports the provided listener. Exporting a listener
	// typically, but not always, involves running a proxy that forwards
	// traffic to the provided address.
	ExportListener(context.Context, *protos.ExportListenerRequest) (*protos.ExportListenerReply, error)
}
