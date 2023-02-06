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

package status

import (
	"context"
	"fmt"
	"syscall"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	protos "github.com/ServiceWeaver/weaver/runtime/protos"
)

// fakeClient is a fake Server that returns the provided status and error.
type fakeClient struct {
	status *Status // returned by the Status method
	err    error   // returned by the Status method
}

// Status implements the Server interface.
func (f fakeClient) Status(context.Context) (*Status, error) {
	return f.status, f.err
}

// Metrics implements the Server interface.
func (f fakeClient) Metrics(context.Context) (*Metrics, error) {
	return nil, fmt.Errorf("unimplemented")
}

// Profile implements the Server interface.
func (f fakeClient) Profile(context.Context, *protos.RunProfiling) (*protos.Profile, error) {
	return nil, fmt.Errorf("unimplemented")
}

func TestRegister(t *testing.T) {
	// Create the registry.
	ctx := context.Background()
	registry, err := NewRegistry(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// Register the deployments.
	regs := []Registration{
		{"0", "todo", "localhost:0"},
		{"1", "todo", "localhost:1"},
		{"2", "chat", "localhost:2"},
		{"3", "zardoz", "localhost:3"},
	}
	for _, reg := range regs {
		if err := registry.Register(ctx, reg); err != nil {
			t.Fatalf("%v: %v", reg, err)
		}
	}

	// List the deployments.
	got, err := registry.list()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(regs, got); diff != "" {
		t.Fatalf("List (-want +got):\n%s", diff)
	}
}

func TestUnregister(t *testing.T) {
	// Create the registry.
	ctx := context.Background()
	registry, err := NewRegistry(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// Register the deployments.
	regs := []Registration{
		{"0", "todo", "localhost:0"},
		{"1", "todo", "localhost:1"},
		{"2", "chat", "localhost:2"},
		{"3", "zardoz", "localhost:3"},
	}
	for _, reg := range regs {
		if err := registry.Register(ctx, reg); err != nil {
			t.Fatalf("%v: %v", reg, err)
		}
	}

	// Unregister some deployments.
	for _, reg := range []Registration{regs[0], regs[3]} {
		if err := registry.Unregister(ctx, reg.DeploymentId); err != nil {
			t.Fatalf("%v: %v", reg, err)
		}
	}

	// List the deployments.
	got, err := registry.list()
	if err != nil {
		t.Fatal(err)
	}
	want := []Registration{regs[1], regs[2]}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("List (-want +got):\n%s", diff)
	}
}

func TestListAndGet(t *testing.T) {
	// Create the registry.
	ctx := context.Background()
	registry, err := NewRegistry(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// Fake clients.
	regs := []Registration{
		{"0", "todo", "localhost:0"}, // running
		{"1", "todo", "localhost:1"}, // unregistered
		{"2", "chat", "localhost:2"}, // dead
		{"3", "todo", "localhost:0"}, // superseded
	}
	registry.newClient = func(addr string) Server {
		switch addr {
		case "localhost:0":
			return fakeClient{&Status{DeploymentId: regs[0].DeploymentId}, nil}
		case "localhost:2":
			return fakeClient{nil, syscall.ECONNREFUSED}
		default:
			t.Fatalf("unexpected addr: %q", addr)
			return nil
		}
	}

	// Register the deployments.
	for _, reg := range regs {
		if err := registry.Register(ctx, reg); err != nil {
			t.Fatalf("%v: %v", reg, err)
		}
	}

	// Unregister some deployments.
	if err := registry.Unregister(ctx, regs[1].DeploymentId); err != nil {
		t.Fatalf("%v: %v", regs[1], err)
	}

	// List registrations.
	got, err := registry.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := regs[0:1]
	if diff := cmp.Diff(want, got, protocmp.Transform()); diff != "" {
		t.Fatalf("List (-want +got):\n%s", diff)
	}

	// Successful Gets.
	reg, err := registry.Get(ctx, regs[0].DeploymentId)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(regs[0], reg, protocmp.Transform()); diff != "" {
		t.Fatalf("List (-want +got):\n%s", diff)
	}

	// Unsuccessful Gets.
	for _, deploymentId := range []string{
		regs[1].DeploymentId,
		regs[2].DeploymentId,
		regs[3].DeploymentId,
		"not a valid deployment id",
	} {
		if _, err := registry.Get(ctx, deploymentId); err == nil {
			t.Fatalf("Get(%q): unexpected success", deploymentId)
		}
	}
}
