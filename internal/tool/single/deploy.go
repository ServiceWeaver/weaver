// Copyright 2023 Google LLC
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

package single

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/tool"
)

var deployCmd = tool.Command{
	Name:        "deploy",
	Description: "Deploy a Service Weaver app",
	Help:        "Usage:\n  weaver single deploy <configfile>",
	Flags:       flag.NewFlagSet("deploy", flag.ContinueOnError),
	Fn:          deploy,
}

// deploy deploys an application on the local machine using the single process
// deployer.
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
	config, err := runtime.ParseConfig(configFile, string(bytes), codegen.ComponentConfigValidator)
	if err != nil {
		return fmt.Errorf("load config file %q: %w\n", configFile, err)
	}
	if _, err := os.Stat(config.Binary); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("binary %q doesn't exist", config.Binary)
	}

	// Set up the binary.
	cmd := exec.Command(config.Binary)
	cmd.Args = config.Args
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Make sure that the subprocess dies when we die. This isn't perfect, as
	// we can't catch a SIGKILL, but it's good in the common case.
	killed := make(chan os.Signal, 1)
	signal.Notify(killed, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-killed
		if cmd.Process != nil {
			cmd.Process.Kill() //nolint:errcheck
		}
		os.Exit(1)
	}()

	// Run the binary
	return cmd.Run()
}
