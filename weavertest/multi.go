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
	"sync"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/babysitter"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/logging"
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

	// TODO: Pass config through weaver.Init. Current plan is to pass a
	// a value in context, and also use that approach to get rid of environment
	// hackery we are currently doing in babysitter, which will make it so that
	// weavertest tests can run in parallel without interfering with each other.
	var mu sync.Mutex
	stopLog := false
	ctx, cancelFunc := context.WithCancel(ctx)
	logSaver := func(e *protos.LogEntry) {
		// TODO(mwhittaker): Capture the main process' log output and report it
		// using t.Log as well.
		mu.Lock()
		defer mu.Unlock()
		if !stopLog {
			e.TimeMicros = time.Now().UnixMicro()
			t.Log(logging.NewPrettyPrinter(colors.Enabled()).Format(e))
		}
	}
	dep := createDeployment(t, config)
	b, err := babysitter.NewBabysitter(ctx, dep, logSaver)
	if err != nil {
		t.Fatal(err)
	}

	// Deploy main.
	toWeavelet, toEnvelope, err := b.CreateAndRunEnvelopeForMain(createWeaveletForMain(dep), dep.App)
	if err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, runtime.BootstrapKey{}, runtime.Bootstrap{
		ToWeaveletFd: toWeavelet,
		ToEnvelopeFd: toEnvelope,
		TestConfig:   config,
	})

	t.Cleanup(func() {
		cancelFunc()
		maybeLogStacks()
		mu.Lock()
		stopLog = true
		mu.Unlock()
	})

	return weaver.Init(ctx)
}

func createDeployment(t testing.TB, config string) *protos.Deployment {
	// Parse supplied config, if any.
	appConfig := &protos.AppConfig{}
	if config != "" {
		var err error
		appConfig, err = runtime.ParseConfig("[testconfig]", config, codegen.ComponentConfigValidator)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Overwrite app config with true executable info.
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("error fetching binary path: %v", err)
	}

	appName := strings.ReplaceAll(t.Name(), "/", "_")
	appConfig.Name = appName
	appConfig.Binary = exe
	// TODO: Forward os.Args[1:] as well?
	appConfig.Args = []string{"-test.run", regexp.QuoteMeta(t.Name())}
	dep := &protos.Deployment{
		Id:  uuid.New().String(),
		App: appConfig,
	}
	return dep
}

func createWeaveletForMain(dep *protos.Deployment) *protos.WeaveletInfo {
	group := &protos.ColocationGroup{
		Name: "main",
	}
	wlet := &protos.WeaveletInfo{
		App:           dep.App.Name,
		DeploymentId:  dep.Id,
		Group:         group,
		GroupId:       uuid.New().String(),
		Id:            uuid.New().String(),
		SameProcess:   dep.App.SameProcess,
		Sections:      dep.App.Sections,
		SingleProcess: dep.SingleProcess,
	}
	return wlet
}
