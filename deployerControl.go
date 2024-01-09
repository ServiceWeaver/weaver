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

// LogBatch implements the control.DeployerControl interface.
func (local *localDeployerControl) LogBatch(ctx context.Context, batch *protos.LogEntryBatch) error {
	for _, entry := range batch.Entries {
		fmt.Fprintln(os.Stderr, local.pp.Format(entry))
	}
	return nil
}

// ActivateComponent implements the control.DeployerControl interface.
func (*localDeployerControl) ActivateComponent(context.Context, *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error) {
	return nil, fmt.Errorf("localDeployerControl.ActivateComponent not implemented")
}

// GetListenerAddress implements the control.DeployerControl interface.
func (*localDeployerControl) GetListenerAddress(context.Context, *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
	return nil, fmt.Errorf("localDeployerControl.GetListenerAddress not implemented")
}

// ExportListener implements the control.DeployerControl interface.
func (*localDeployerControl) ExportListener(context.Context, *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	return nil, fmt.Errorf("localDeployerControl.ExportListener not implemented")
}

// GetSelfCertificate implements the control.DeployerControl interface.
func (*localDeployerControl) GetSelfCertificate(context.Context, *protos.GetSelfCertificateRequest) (*protos.GetSelfCertificateReply, error) {
	return nil, fmt.Errorf("localDeployerControl.GetSelfCertificate not implemented")
}

// VerifyClientCertificate implements the control.DeployerControl interface.
func (*localDeployerControl) VerifyClientCertificate(context.Context, *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error) {
	return nil, fmt.Errorf("localDeployerControl.VerifyClientCertificate not implemented")
}

// VerifyServerCertificate implements the control.DeployerControl interface.
func (*localDeployerControl) VerifyServerCertificate(context.Context, *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error) {
	return nil, fmt.Errorf("localDeployerControl.VerifyServerCertificate not implemented")
}
