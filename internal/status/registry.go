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
	// registration r is stored in a JSON file called
	// {r.DeploymentId}.registration.json.
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
		return nil, fmt.Errorf("registry: filepath.Abs(%q): %w", dir, err)
	}
	if err = os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("registry: make dir %q: %w", dir, err)
	}
	newClient := func(addr string) Server { return NewClient(addr) }
	return &Registry{dir, newClient}, nil
}

// Register adds a registration to the registry.
func (r *Registry) Register(ctx context.Context, reg Registration) error {
	bytes, err := json.Marshal(reg)
	if err != nil {
		return fmt.Errorf("registry: encode %v: %w", reg, err)
	}
	filename := fmt.Sprintf("%s.registration.json", reg.DeploymentId)
	filename = filepath.Join(r.dir, filename)
	w := files.NewWriter(filename)
	defer w.Cleanup()
	if _, err := w.Write(bytes); err != nil {
		return fmt.Errorf("registry: write %q: %w", filename, err)
	}
	return w.Close()
}

// Unregister removes a registration from the registry.
func (r *Registry) Unregister(_ context.Context, deploymentId string) error {
	filename := fmt.Sprintf("%s.registration.json", deploymentId)
	filename = filepath.Join(r.dir, filename)
	if err := os.Remove(filename); err != nil {
		return fmt.Errorf("registry: remove %q: %w", filename, err)
	}
	return nil
}

// Get returns the Registration for the provided deployment. If the deployment
// doesn't exist or is not active, a non-nil error is returned.
func (r *Registry) Get(ctx context.Context, deploymentId string) (Registration, error) {
	// TODO(mwhittaker): r.list() reads and parses every registration file.
	// This is inefficient, as we could instead stop reading and parsing as
	// soon as we find the corresponding registration file. Even more
	// efficient, we could match the deploymentId to the filenames instead of
	// reading and parsing the files. Since the number of registrations is
	// small, and the size of every registration file is small, I don't think
	// these optimizations are urgently needed.
	regs, err := r.list()
	if err != nil {
		return Registration{}, err
	}
	for _, reg := range regs {
		if reg.DeploymentId != deploymentId {
			continue
		}
		if r.dead(ctx, reg) {
			return Registration{}, fmt.Errorf("registry: deployment %q not found", deploymentId)
		}
		return reg, nil
	}
	return Registration{}, fmt.Errorf("registry: deployment %q not found", deploymentId)
}

// List returns all active Registrations.
func (r *Registry) List(ctx context.Context) ([]Registration, error) {
	regs, err := r.list()
	if err != nil {
		return nil, err
	}
	var alive []Registration
	for _, reg := range regs {
		// When a Service Weaver application is deployed, it registers itself
		// with a registry. Ideally, the deployment would also unregister
		// itself when it terminates, but this is hard to guarantee. If a
		// deployment is killed abruptly, via `kill -9` for example, then the
		// deployment may not unregister itself.
		//
		// Thus, we need a way to detect stale registrations (i.e.
		// registrations for deployments that have terminated) and garbage
		// collect them. We do this by curling a deployment's status server. If
		// the server is dead, we consider the deployment dead, and we garbage
		// collect its registration.
		if r.dead(ctx, reg) {
			if err := r.Unregister(ctx, reg.DeploymentId); err != nil {
				return nil, err
			}
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
		return nil, fmt.Errorf("registry: read dir %q: %w", r.dir, err)
	}

	var regs []Registration
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".registration.json") {
			// Ignore non-registration files in the registry directory.
			continue
		}
		filename := filepath.Join(r.dir, entry.Name())
		bytes, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("registry: read file %q: %w", filename, err)
		}
		var reg Registration
		if err := json.Unmarshal(bytes, &reg); err != nil {
			return nil, fmt.Errorf("registry: decode file %q: %w", filename, err)
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
	case errors.Is(err, syscall.Errno(10061)):
		// The syscall.ECONNREFUSED doesn't work on Windows. Windows will
		// return WSAECONNREFUSED(syscall.Errno = 10061) when the connection is
		// refused.
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
