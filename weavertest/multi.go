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
	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
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
		t.Helper()
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

	// Set up the pipes between the envelope and the main weavelet. The
	// pipes will be closed by the envelope and weavelet conns.
	//
	//         envelope                      weavelet
	//         --------        +----+        --------
	//   fromWeaveletReader <--| OS |<-- fromWeaveletWriter
	//     toWeaveletWriter -->|    |--> toWeaveletReader
	//                         +----+
	fromWeaveletReader, fromWeaveletWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(fmt.Errorf("cannot create fromWeavelet pipe: %v", err))
	}
	toWeaveletReader, toWeaveletWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(fmt.Errorf("cannot create toWeavelet pipe: %v", err))
	}

	weaveletInfo := createWeaveletForMain(dep)

	// Create a connection between the weavelet for the main process and the envelope.
	econn, err := conn.NewEnvelopeConn(fromWeaveletReader, toWeaveletWriter, b, weaveletInfo)
	if err != nil {
		t.Fatal(fmt.Errorf("cannot create envelope conn: %v", err))
	}

	go func() {
		if err := econn.Run(); err != nil {
			t.Error(err)
		}
	}()

	// There is a very subtle bug we have to avoid. As explained in the
	// official godoc [1], if an *os.File is garbage collected, the underlying
	// file is closed. Thus, if toWeaveletReader or fromWeaveletWriter are
	// garbage collected, the underlying pipes are closed.
	//
	// The main weavelet, which is running in a separate goroutine, is reading
	// from and writing to these pipes, but it doesn't use toWeaveletReader or
	// fromWeaveletWriter directly. Instead components to start., it constructs new files using the
	// two pipe's file descriptors. This means that Go is free to garbage
	// collect fromWeaveletWriter and toWeaveletWriter even though the
	// underlying pipes are still being used by the main weavelet.
	//
	// If the pipes get closed while the main weavelet is running, then the
	// weavelet can crash with all sorts of errors (e.g., bad pipe, resource
	// temporarily unavailable, nil pointer dereferences, etc). You can run [2]
	// locally to reproduce this behavior. If the program doesn't crash on your
	// machine, try increasing the number of iterations.
	//
	// To avoid this, we have to ensure that the pipes are not garbage
	// collected prematurely. For now, we place them in a global variable. It's
	// not a great solution, but it gets the job done.
	//
	// [1]: https://pkg.go.dev/os#File.Fd
	// [2]: https://go.dev/play/p/9JG7voL2oHP
	dontGCLock.Lock()
	dontGC = append(dontGC, fromWeaveletReader, fromWeaveletWriter,
		toWeaveletReader, toWeaveletWriter)
	dontGCLock.Unlock()

	t.Cleanup(func() {
		cancelFunc()

		maybeLogStacks()
		mu.Lock()
		stopLog = true
		mu.Unlock()
	})

	initCtx := context.WithValue(ctx, runtime.BootstrapKey{}, runtime.Bootstrap{
		ToWeaveletFd: int(toWeaveletReader.Fd()),
		ToEnvelopeFd: int(fromWeaveletWriter.Fd()),
		TestConfig:   config,
	})
	return weaver.Init(initCtx)
}

// See comment in initMultiProcess.
var (
	dontGCLock sync.Mutex
	dontGC     []*os.File
)

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
