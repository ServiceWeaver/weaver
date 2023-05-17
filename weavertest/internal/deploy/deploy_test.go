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

package deploy_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ServiceWeaver/weaver/weavertest"
	"github.com/ServiceWeaver/weaver/weavertest/internal/deploy"
)

// TestReplicated tests that concurrent deployers deploy only a single instance
// of each replica of a replicated process.
func TestReplicated(t *testing.T) {
	ctx := context.Background()

	// Instruct the weavertest deployer to replicate each component. Note that each
	// component should be replicated weavertest.DefaultReplication times.
	weavertest.Multi.Run(t, func(w deploy.Widget) {
		dir := t.TempDir()
		w.Use(ctx, dir)
		// Verify that deployed processes started.
		want := weavertest.DefaultReplication
		if got := numDeployed(t, dir); got != want {
			t.Fatalf("wrong number of deployed processes: want %d, got %d", want, got)
		}
	})
}

// numDeployed returns the number of Started or ReplicatedStarted components that
// have been successfully deployed.
func numDeployed(t *testing.T, dir string) int {
	t.Helper()
	// To get the count of successfully deployed processes, we count the number
	// of files with suffix "started" in dir.
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("os.ReadDir: %v", err)
	}
	numDeployed := 0
	for _, f := range files {
		if strings.HasSuffix(f.Name(), "started") {
			numDeployed++
		}
	}
	return numDeployed
}
