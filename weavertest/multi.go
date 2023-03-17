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

package weavertest

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
)

// initMultiProcess initializes a brand new multi-process execution environment
// that places every component in its own collocation group and returns the root
// component for the new application.
//
// config contains configuration identical to what might be found in a file passed
// when deploying an application. It can contain application level as well as
// component level configs. config is allowed to be empty.
//
// Future extension: allow options so the user can control collocation/replication/etc.
func initMultiProcess(ctx context.Context, t testing.TB, config string) weaver.Instance {
	t.Helper()
	bootstrap, err := runtime.GetBootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.HasPipes() {
		// This is a child process, so just call weaver.Init() which should block.
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "panic in Service Weaver sub-process: %v\n", r)
			} else {
				fmt.Fprintf(os.Stderr, "Service Weaver sub-process exiting\n")
			}
			os.Exit(1)
		}()
		weaver.Init(context.Background())
		return nil
	}

	// Construct AppConfig and WeaveletInfo.
	appConfig := &protos.AppConfig{}
	if config != "" {
		var err error
		appConfig, err = runtime.ParseConfig("[testconfig]", config, codegen.ComponentConfigValidator)
		if err != nil {
			t.Fatal(err)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("error fetching binary path: %v", err)
	}
	appConfig.Name = strings.ReplaceAll(t.Name(), "/", "_")
	appConfig.Binary = exe
	appConfig.Args = []string{"-test.run", regexp.QuoteMeta(t.Name())}

	wlet := &protos.WeaveletInfo{
		App:           appConfig.Name,
		DeploymentId:  uuid.New().String(),
		SameProcess:   appConfig.SameProcess,
		Sections:      appConfig.Sections,
		SingleProcess: false,
		SingleMachine: true,
	}

	// Launch the deployer.
	d := newDeployer(ctx, t, wlet, appConfig)
	return d.Init(config)
}
