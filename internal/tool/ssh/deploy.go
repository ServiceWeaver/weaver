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

package ssh

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"

	"github.com/google/uuid"

	"github.com/ServiceWeaver/weaver/internal/tool/ssh/impl"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/tool"
)

var deployCmd = tool.Command{
	Name:        "deploy",
	Description: "Deploy a Service Weaver app",
	Help:        "Usage:\n  weaver ssh deploy <configfile>",
	Flags:       flag.NewFlagSet("deploy", flag.ContinueOnError),
	Fn:          deploy,
}

// deploy deploys an application on a cluster of machines using an SSH deployer.
// Note that each component is deployed as a separate OS process.
func deploy(ctx context.Context, args []string) error {
	// Validate command line arguments.
	if len(args) == 0 {
		return fmt.Errorf("no config file provided")
	}
	if len(args) > 1 {
		return fmt.Errorf("too many arguments")
	}

	// Load the config file.
	cfgFile := args[0]
	cfg, err := os.ReadFile(cfgFile)
	if err != nil {
		return fmt.Errorf("load config file %q: %w", cfgFile, err)
	}
	app, err := runtime.ParseConfig(cfgFile, string(cfg), codegen.ComponentConfigValidator)
	if err != nil {
		return fmt.Errorf("load config file %q: %w", cfgFile, err)
	}

	// Sanity check the config.
	if _, err := os.Stat(app.Binary); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("binary %q doesn't exist", app.Binary)
	}

	// Retrieve the list of locations to deploy.
	locs, err := getLocations(app)
	if err != nil {
		return err
	}

	// Create a deployment.
	dep := &protos.Deployment{
		Id:  uuid.New().String(),
		App: app,
	}

	// Copy the binaries to each location.
	if err := copyBinaries(locs, dep); err != nil {
		return err
	}

	// Run the manager.
	stopFn, err := impl.RunManager(ctx, dep, locs, logDir)
	if err != nil {
		return fmt.Errorf("cannot instantiate the manager: %w", err)
	}

	// Wait for the user to kill the app.
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-done // Will block here until user hits ctrl+c
		if err := terminateDeployment(locs, dep); err != nil {
			fmt.Fprintf(os.Stderr, "failed to terminate deployment: %v\n", err)
		}
		fmt.Fprintf(os.Stderr, "Application %s terminated\n", app.Name)
		if err := stopFn(); err != nil {
			fmt.Fprintf(os.Stderr, "stop the manager: %v\n", err)
		}
		os.Exit(1)
	}()

	// Follow the logs.
	source := logging.FileSource(logDir)
	query := fmt.Sprintf(`full_version == %q && !("serviceweaver/system" in attrs)`, dep.Id)
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

// copyBinaries copies the tool and the application binary to the given set
// of locations.
func copyBinaries(locs []string, dep *protos.Deployment) error {
	ex, err := os.Executable()
	if err != nil {
		return err
	}

	binary := dep.App.Binary
	remoteDepDir := filepath.Join(os.TempDir(), dep.Id)
	dep.App.Binary = filepath.Join(remoteDepDir, filepath.Base(dep.App.Binary))
	for _, loc := range locs {
		// Make an app deployment directory at each location.
		cmd := exec.Command("ssh", loc, "mkdir", "-p", remoteDepDir)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("unable to create deployment directory at location %s: %w\n", loc, err)
		}

		cmd = exec.Command("scp", ex, binary, loc+":"+remoteDepDir)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("unable to copy app binary at location %s: %w\n", loc, err)
		}
	}
	return nil
}

// terminateDeployment terminates all the processes corresponding to the deployment
// at all locations.
//
// TODO(rgrandl): Find a different way to kill the deployment if the pkill command
// is not installed.
func terminateDeployment(locs []string, dep *protos.Deployment) error {
	for _, loc := range locs {
		cmd := exec.Command("ssh", loc, "pkill", "-f", dep.Id)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("unable to terminate deployment at location %s: %w", loc, err)
		}
	}
	return nil
}

// getLocations returns the list of locations at which to deploy the application.
func getLocations(app *protos.AppConfig) ([]string, error) {
	// SSH config as found in TOML config file.
	const sshKey = "github.com/ServiceWeaver/weaver/ssh"
	const shortSSHKey = "ssh"

	type sshConfigSchema struct {
		LocationsFile string `toml:"locations_file"`
	}
	parsed := &sshConfigSchema{}
	if err := runtime.ParseConfigSection(sshKey, shortSSHKey, app.Sections, parsed); err != nil {
		return nil, fmt.Errorf("unable to parse ssh config: %w", err)
	}

	file, err := getAbsoluteFilePath(parsed.LocationsFile)
	if err != nil {
		return nil, err
	}
	readFile, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("unable to open locations file: %w", err)
	}
	defer readFile.Close()

	fileScanner := bufio.NewScanner(readFile)
	fileScanner.Split(bufio.ScanLines)
	var locations []string
	for fileScanner.Scan() {
		locations = append(locations, fileScanner.Text())
	}

	if len(locations) == 0 {
		return nil, fmt.Errorf("no locations to deploy using the ssh deployer")
	}
	return locations, nil
}

// getAbsoluteFilePath returns the absolute path for a file.
func getAbsoluteFilePath(file string) (string, error) {
	if len(file) == 0 {
		return "", fmt.Errorf("file not specified")
	}
	if file[0] == '~' {
		usr, err := user.Current()
		if err != nil {
			return "", err
		}
		return filepath.Join(usr.HomeDir, file[1:]), nil
	}
	// Getting absolute path of the file.
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", fmt.Errorf("unable to find file %s: %w", file, err)
	}
	return abs, nil
}
