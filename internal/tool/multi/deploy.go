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

package multi

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ServiceWeaver/weaver/internal/status"
	"github.com/ServiceWeaver/weaver/internal/tool/config"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/bin"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"github.com/ServiceWeaver/weaver/runtime/tool"
	"github.com/ServiceWeaver/weaver/runtime/version"
	"github.com/google/uuid"
)

const (
	configKey      = "github.com/ServiceWeaver/weaver/multi"
	shortConfigKey = "multi"
)

var deployCmd = tool.Command{
	Name:        "deploy",
	Description: "Deploy a Service Weaver app",
	Help:        "Usage:\n  weaver multi deploy <configfile>",
	Flags:       flag.NewFlagSet("deploy", flag.ContinueOnError),
	Fn:          deploy,
}

// deploy deploys an application on the local machine using a multiprocess
// deployer. Note that each component is deployed as a separate OS process.
func deploy(ctx context.Context, args []string) error {
	// Validate command line arguments.
	if len(args) == 0 {
		return fmt.Errorf("no config file provided")
	}
	if len(args) > 1 {
		return fmt.Errorf("too many arguments")
	}

	// Load the config file.
	configFile := args[0]
	bytes, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("load config file %q: %w\n", configFile, err)
	}

	// Parse and sanity-check the application section of the config.
	appConfig, err := runtime.ParseConfig(configFile, string(bytes), codegen.ComponentConfigValidator)
	if err != nil {
		return fmt.Errorf("load config file %q: %w\n", configFile, err)
	}
	if _, err := os.Stat(appConfig.Binary); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("binary %q doesn't exist", appConfig.Binary)
	}

	// Parse the multi section of the config.
	multiConfig, err := config.GetDeployerConfig[MultiConfig, MultiConfig_ListenerOptions](configKey, shortConfigKey, appConfig)
	if err != nil {
		return err
	}
	multiConfig.App = appConfig

	// Check version compatibility.
	versions, err := bin.ReadVersions(appConfig.Binary)
	if err != nil {
		return fmt.Errorf("read versions: %w", err)
	}
	if versions.DeployerVersion != version.DeployerVersion {
		// Try to relativize the binary, defaulting to the absolute path if
		// there are any errors..
		binary := appConfig.Binary
		if cwd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(cwd, appConfig.Binary); err == nil {
				binary = rel
			}
		}
		return fmt.Errorf(`
ERROR: The binary you're trying to deploy (%q) was built with
github.com/ServiceWeaver/weaver module version %s. However, the 'weaver
multi' binary you're using was built with weaver module version %s.
These versions are incompatible.

We recommend updating both the weaver module your application is built with and
updating the 'weaver multi' command by running the following.

    go get github.com/ServiceWeaver/weaver@latest
    go install github.com/ServiceWeaver/weaver/cmd/weaver@latest

Then, re-build your code and re-run 'weaver multi deploy'. If the problem
persists, please file an issue at https://github.com/ServiceWeaver/weaver/issues.`,
			binary, versions.ModuleVersion, version.ModuleVersion)
	}

	// Create the deployer.
	deploymentId := uuid.New().String()
	d, err := newDeployer(ctx, deploymentId, multiConfig)
	if err != nil {
		return fmt.Errorf("create deployer: %w", err)
	}

	// Start signal handler before listener
	userDone := make(chan os.Signal, 1)
	signal.Notify(userDone, syscall.SIGINT, syscall.SIGTERM)

	// Run a status server.
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	mux := http.NewServeMux()
	status.RegisterServer(mux, d, d.logger)
	go func() {
		if err := serveHTTP(ctx, lis, mux); err != nil {
			fmt.Fprintf(os.Stderr, "status server: %v\n", err)
		}
	}()

	// Deploy main.
	if err := d.startMain(); err != nil {
		return fmt.Errorf("start main process: %w", err)
	}

	// Wait for the status server to become active.
	client := status.NewClient(lis.Addr().String())
	for r := retry.Begin(); r.Continue(ctx); {
		_, err := client.Status(ctx)
		if err == nil {
			break
		}
		fmt.Fprintf(os.Stderr, "status server %q unavailable: %#v\n", lis.Addr(), err)
	}

	// Register the deployment.
	registry, err := defaultRegistry(ctx)
	if err != nil {
		return fmt.Errorf("create registry: %w", err)
	}
	reg := status.Registration{
		DeploymentId: deploymentId,
		App:          appConfig.Name,
		Addr:         lis.Addr().String(),
	}
	fmt.Fprint(os.Stderr, reg.Rolodex())
	if err := registry.Register(ctx, reg); err != nil {
		return fmt.Errorf("register deployment: %w", err)
	}

	deployerDone := make(chan error, 1)
	go func() {
		err := d.wait()
		deployerDone <- err
	}()
	go func() {
		var code = 1
		// Wait for the user to kill the app or the app to return an error.
		select {
		case sig := <-userDone:
			fmt.Fprintf(os.Stderr, "Application %s terminated by the user\n", appConfig.Name)
			code = 128 + int(sig.(syscall.Signal))
		case err := <-deployerDone:
			fmt.Fprintf(os.Stderr, "Application %s error: %v\n", appConfig.Name, err)
		}
		if err := registry.Unregister(ctx, deploymentId); err != nil {
			fmt.Fprintf(os.Stderr, "unregister deployment: %v\n", err)
			code = 1
		}
		os.Exit(code)
	}()

	// Follow the logs.
	source := logging.FileSource(logDir)
	query := fmt.Sprintf(`full_version == %q && !("serviceweaver/system" in attrs)`, deploymentId)
	r, err := source.Query(ctx, query, true)
	if err != nil {
		return err
	}
	pp := logging.NewPrettyPrinter(colors.Enabled())
	for {
		entry, err := r.Read(ctx)
		if errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
		fmt.Println(pp.Format(entry))
	}
}

// defaultRegistry returns a registry in defaultRegistryDir().
func defaultRegistry(ctx context.Context) (*status.Registry, error) {
	return status.NewRegistry(ctx, registryDir)
}
