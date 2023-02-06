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

// Package status contains code for implementing status related commands like
// "weaver multi status" and "weaver single dashboard".
package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ServiceWeaver/weaver/internal/files"
	"github.com/ServiceWeaver/weaver/runtime/colors"
)

// A Registry is a persistent collection of Service Weaver application metadata.
//
// Tools like "weaver multi status" and "weaver multi dashboard" use the registry
// to know which applications are running and to fetch the status of the
// running applications.
type Registry struct {
	// A Registry stores registrations as files in a directory. Every
	// registration r is stored in a JSON file called {r.DeploymentId}.json.
	//
	// TODO(mwhittaker): Store as protos instead of JSON?
	dir string

	// newClient returns a new status client that curls the provided address.
	// It is a field of Registry to enable dependency injection in
	// registry_test.go.
	newClient func(string) Server
}

// A Registration contains basic metadata about a Service Weaver application.
type Registration struct {
	DeploymentId string // deployment id (e.g, "eba18295")
	App          string // app name (e.g., "todo")
	Addr         string // status server (e.g., "localhost:12345")
}

// Rolodex returns a pretty-printed rolodex displaying the registration.
//
//	╭───────────────────────────────────────────────────╮
//	│ app        : collatz                              │
//	│ deployment : fdeeb059-825b-4606-9e99-e22e63e10552 │
//	╰───────────────────────────────────────────────────╯
func (r Registration) Rolodex() string {
	// Declare the contents.
	type kv struct {
		key string
		val colors.Text
	}
	prefix, suffix := formatId(r.DeploymentId)
	kvs := []kv{
		{"app        :", colors.Text{colors.Atom{S: r.App}}},
		{"deployment :", colors.Text{prefix, suffix}},
	}

	length := func(t colors.Text) int {
		var n int
		for _, a := range t {
			n += len(a.S)
		}
		return n
	}

	// Calculate widths.
	valWidth := 0
	for _, kv := range kvs {
		if length(kv.val) > valWidth {
			valWidth = length(kv.val)
		}
	}
	width := valWidth + len(kvs[0].key) + 5

	// Pretty print.
	var b strings.Builder
	fmt.Fprintf(&b, "╭%s╮\n", strings.Repeat("─", width-2))
	for _, kv := range kvs {
		s := kv.val.String()
		fmt.Fprintf(&b, "│ %s %-*s │\n", kv.key, valWidth+len(s)-length(kv.val), s)
	}
	fmt.Fprintf(&b, "╰%s╯\n", strings.Repeat("─", width-2))
	return b.String()
}

// NewRegistry returns a registry that persists data to the provided directory.
func NewRegistry(_ context.Context, dir string) (*Registry, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	err = os.MkdirAll(dir, 0750)
	newClient := func(addr string) Server { return NewClient(addr) }
	return &Registry{dir, newClient}, err
}

// DefaultRegistryDir returns the default registry directory
// $XDG_DATA_HOME/serviceweaver, or ~/.local/share/serviceweaver if
// XDG_DATA_HOME is not set.
func DefaultRegistryDir() (string, error) {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		// Default to ~/.local/share
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataDir = filepath.Join(home, ".local", "share")
	}
	regDir := filepath.Join(dataDir, "serviceweaver")
	if err := os.MkdirAll(regDir, 0700); err != nil {
		return "", err
	}
	return regDir, nil
}

// Register adds a registration to the registry.
func (r *Registry) Register(ctx context.Context, reg Registration) error {
	bytes, err := json.Marshal(reg)
	if err != nil {
		return err
	}
	filename := fmt.Sprintf("%s.json", reg.DeploymentId)
	w := files.NewWriter(filepath.Join(r.dir, filename))
	defer w.Cleanup()
	if _, err := w.Write(bytes); err != nil {
		return err
	}
	return w.Close()
}

// Unregister removes a registration from the registry.
func (r *Registry) Unregister(_ context.Context, deploymentId string) error {
	filename := fmt.Sprintf("%s.json", deploymentId)
	return os.Remove(filepath.Join(r.dir, filename))
}

// Get returns the Registration for the provided deployment. If the deployment
// doesn't exist or is not active, a non-nil error is returned.
func (r *Registry) Get(ctx context.Context, deploymentId string) (Registration, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return Registration{}, err
	}
	for _, entry := range entries {
		bytes, err := os.ReadFile(filepath.Join(r.dir, entry.Name()))
		if err != nil {
			return Registration{}, err
		}
		var reg Registration
		if err := json.Unmarshal(bytes, &reg); err != nil {
			return Registration{}, err
		}
		if reg.DeploymentId != deploymentId {
			continue
		}
		if r.dead(ctx, reg) {
			return Registration{}, fmt.Errorf("deployment %q not found", deploymentId)
		}
		return reg, nil
	}
	return Registration{}, fmt.Errorf("deployment %q not found", deploymentId)
}

// List returns all active Registrations.
func (r *Registry) List(ctx context.Context) ([]Registration, error) {
	regs, err := r.list()
	if err != nil {
		return nil, err
	}
	var alive []Registration
	for _, reg := range regs {
		// When a Service Weaver application is deployed, it registers itself with a
		// registry. Ideally, the deployment would also unregister itself when
		// it terminates, but this is hard to guarantee. If a deployment is
		// killed abruptly, via `kill -9` for example, then the deployment may
		// not unregister itself. Single process deployments (deployed via `go
		// run .`) also do not unregister themselves.
		//
		// Thus, we need a way to detect stale registrations (i.e.
		// registrations for deployments that have terminated) and garbage
		// collect them. We do this by curling a deployment's status server. If
		// the server is dead, we consider the deployment dead, and we garbage
		// collect its registration.
		if r.dead(ctx, reg) {
			r.Unregister(ctx, reg.DeploymentId)
			continue
		}
		alive = append(alive, reg)
	}
	return alive, nil
}

// list returns all registrations, dead or alive.
func (r *Registry) list() ([]Registration, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, err
	}

	var regs []Registration
	for _, entry := range entries {
		bytes, err := os.ReadFile(filepath.Join(r.dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var reg Registration
		if err := json.Unmarshal(bytes, &reg); err != nil {
			return nil, err
		}
		regs = append(regs, reg)
	}
	return regs, nil
}

// dead returns whether the provided registration is associated with a
// deployment that is definitely dead.
func (r *Registry) dead(ctx context.Context, reg Registration) bool {
	status, err := r.newClient(reg.Addr).Status(ctx)
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		// There is no status server for this deployment, so we consider
		// the deployment dead.
		return true
	case err != nil:
		// Something went wrong. The deployment may be dead, but we're not 100%
		// sure, so we return false.
		return false
	case status.DeploymentId != reg.DeploymentId:
		// The status server for this deployment is dead and has been
		// superseded by a newer status server.
		return true
	default:
		return false
	}
}
