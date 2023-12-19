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

package control

import (
	"context"

	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// Controller is the interface for the weaver.controller component. It is present in
// its own package so other packages do not need to copy the interface definition.
//
// Arguments and results are protobufs to allow deployers to evolve independently of
// application binaries.
type Controller interface {
	// UpdateComponents updates the weavelet with the latest set of components it
	// should be running.
	UpdateComponents(context.Context, *protos.UpdateComponentsRequest) (*protos.UpdateComponentsReply, error)

	// UpdateRoutingInfo updates the weavelet with a component's most recent routing info.
	UpdateRoutingInfo(context.Context, *protos.UpdateRoutingInfoRequest) (*protos.UpdateRoutingInfoReply, error)
}
