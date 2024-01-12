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

	"github.com/ServiceWeaver/weaver/internal/control"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// weaveletControl is a component hosted in every weavelet. Deployers make calls to this component
// to fetch information about the weavelet, and to make it do various things.
type weaveletControl control.WeaveletControl

// noopWeaveletControl is a no-op implementation of weaveletControl. It exists solely to cause
// weaveletControl to be registered as a component. The actual implementation is provided
// by internal/weaver/remoteweavelet.go
type noopWeaveletControl struct {
	Implements[weaveletControl]
}

var _ weaveletControl = &noopWeaveletControl{}

// InitWeavelet implements weaveletControl interface.
func (*noopWeaveletControl) InitWeavelet(context.Context, *protos.InitWeaveletRequest) (*protos.InitWeaveletReply, error) {
	return nil, fmt.Errorf("weaveletControl.InitWeavelet not implemented")
}

// UpdateComponents implements weaveletControl interface.
func (*noopWeaveletControl) UpdateComponents(context.Context, *protos.UpdateComponentsRequest) (*protos.UpdateComponentsReply, error) {
	return nil, fmt.Errorf("weaveletControl.UpdateComponents not implemented")
}

// UpdateRoutingInfo implements weaveletControl interface.
func (*noopWeaveletControl) UpdateRoutingInfo(context.Context, *protos.UpdateRoutingInfoRequest) (*protos.UpdateRoutingInfoReply, error) {
	return nil, fmt.Errorf("weaveletControl.UpdateRoutingInfo not implemented")
}

// GetHealth implements weaveletControl nterface.
func (*noopWeaveletControl) GetHealth(context.Context, *protos.GetHealthRequest) (*protos.GetHealthReply, error) {
	return nil, fmt.Errorf("weaveletControl.GetHealth not implemented")
}

// GetLoad implements weaveletControl nterface.
func (*noopWeaveletControl) GetLoad(context.Context, *protos.GetLoadRequest) (*protos.GetLoadReply, error) {
	return nil, fmt.Errorf("weaveletControl.GetLoad not implemented")
}

// GetMetrics implements weaveletControl nterface.
func (*noopWeaveletControl) GetMetrics(context.Context, *protos.GetMetricsRequest) (*protos.GetMetricsReply, error) {
	return nil, fmt.Errorf("weaveletControl.GetMetrics not implemented")
}

// GetProfile implements weaveletControl nterface.
func (*noopWeaveletControl) GetProfile(context.Context, *protos.GetProfileRequest) (*protos.GetProfileReply, error) {
	return nil, fmt.Errorf("weaveletControl.GetProfile not implemented")
}
