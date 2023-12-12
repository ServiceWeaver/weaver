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
	"context"
	"fmt"

	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// controller is a component hosted in every weavelet. Deployers make calls to this component to
// fetch information about the weavelet, and to make it do various things.
//
// Arguments and results are protobufs to allow deployers to evolve independently of application
// binaries.
type controller interface {
	// GetHealth returns the health of the weavelet.
	GetHealth(context.Context, *protos.GetHealthRequest) (*protos.GetHealthReply, error)

	// UpdateComponents updates the weavelet with the latest set of components it
	// should be running.
	UpdateComponents(context.Context, *protos.UpdateComponentsRequest) (*protos.UpdateComponentsReply, error)
}

// noopController is a no-op implementation of controller. It exists solely to cause
// controller to be registered as a component. The actual implementation is provided
// by internal/weaver/remoteweavelet.go
type noopController struct {
	Implements[controller]
}

var _ controller = &noopController{}

// GetHealth implements controller interface.
func (*noopController) GetHealth(context.Context, *protos.GetHealthRequest) (*protos.GetHealthReply, error) {
	return nil, fmt.Errorf("controller.GetHealth not implemented")
}

// UpdateComponents implements controller interface.
func (*noopController) UpdateComponents(context.Context, *protos.UpdateComponentsRequest) (*protos.UpdateComponentsReply, error) {
	return nil, fmt.Errorf("controller.UpdateComponents not implemented")
}
