// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package weaver

import (
	"context"
	"reflect"

	"github.com/ServiceWeaver/weaver/internal/weaver"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/grpcregistry"
	"google.golang.org/grpc"
)

func RegisterComponent[C any](client func(grpc.ClientConnInterface) C, server func(s grpc.ServiceRegistrar) error) {
	grpcregistry.Register(grpcregistry.Registration{
		ClientType: reflect.TypeOf((*C)(nil)).Elem(),
		Client: func(cc grpc.ClientConnInterface) any {
			return client(cc)
		},
		Server: server,
	})
}

// GetClient - returns a grpc client corresponding to a grpc server that run in
// a colocation group. The grpc client should load balance requests across all
// the server replicas.
func GetClient[T any]() (T, error) {
	h, err := grpcregistry.GetGrpcClient(reflect.TypeOf((*T)(nil)).Elem())
	if err != nil {
		var x T
		return x, err
	}
	return h.(T), err
}

type Registration[C any] struct {
	Name   string
	Client func(cc grpc.ClientConnInterface) C
	Server func(s grpc.ServiceRegistrar) error
}

//////////////////////////// END NEW APIs //////////////////////////////

// Registration as passed by the user code.

// RunGrpc - similar to weaver.Run, runs an app.
func RunGrpc(ctx context.Context, app func(ctx context.Context) error) error {
	// Decides whether to run locally or remote.
	bootstrap, err := runtime.GetBootstrap(ctx)
	if err != nil {
		return err
	}
	if !bootstrap.Exists() {
		// Run the app in a single process.
		return runGrpcLocal(ctx, app)
	}
	// Otherwise, run the local distributed.
	return runGrpcRemote(ctx, app, bootstrap)
}

func runGrpcLocal(ctx context.Context, app func(ctx context.Context) error) error {
	components := grpcregistry.Registered()

	wlet, err := weaver.NewSingleGrpcWeavelet(ctx, components)
	if err != nil {
		return err
	}

	// Return when either (1) the remote weavelet exits, or (2) the user provided
	// app function returns, whichever happens first.
	errs := make(chan error, 2)
	go func() {
		errs <- app(ctx)
	}()
	go func() {
		errs <- wlet.Wait()
	}()
	return <-errs
}

func runGrpcRemote(ctx context.Context, app func(ctx context.Context) error, bootstrap runtime.Bootstrap) error {
	components := grpcregistry.Registered()

	wlet, err := weaver.NewRemoteWeaveletGrpc(ctx, components, bootstrap)
	if err != nil {
		return err
	}

	// Return when either (1) the remote weavelet exits, or (2) the user provided
	// app function returns, whichever happens first.
	errs := make(chan error, 2)
	if wlet.Info().RunMain {
		go func() {
			errs <- app(ctx)
		}()
	}
	go func() {
		errs <- wlet.Wait()
	}()
	return <-errs
}
