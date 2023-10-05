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

// Package exec implements the "weaver exec" command.
package exec

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/internal/run"
)

const usage = `Execute a Service Weaver application.

Usage:
  weaver exec <config file> <topology file> <node>

Flags:
  -h, --help	Print this help message.

Description:
  "weaver exec" is low-level command to run a Service Weaver application when
  the topology of the application (e.g., the address of every listener and
  component) is static and predetermined.

  It is more convenient to deploy an application using a deployer like "weaver
  multi" or "weaver kube". "weaver exec" gives you more control over how you
  deploy your application, but this comes at the cost of being more cumbersome
  to use.

Topology Files:
  To deploy an application using "weaver exec", you must specify the topology
  of your application in a TOML file. For example, consider the following
  topology file for a Service Weaver application with components Main, Foo, and
  Bar where the Main component exports a listener named lis.

      ┌────────────────────topology.toml──────────────────────┐
      │ id = "v1"                                             │
      │                                                       │
      │ [nodes.main]                                          │
      │ address = "localhost:9000"                            │
      │ components = ["github.com/ServiceWeaver/weaver/Main"] │
      │ listeners.lis = "localhost:8000"                      │
      │                                                       │
      │ [nodes.foobar]                                        │
      │ address = ":9001"                                     │
      │ dial_address = "localhost:9001"                       │
      │ components = [                                        │
      │     "github.com/example/app/Foo",                     │
      │     "github.com/example/app/Bar",                     │
      │ ]                                                     │
      └───────────────────────────────────────────────────────┘

  A topology file begins with a unique deployment id (e.g., "v1"). Next is a
  series of node definitions. Every node has a name (e.g., "main", "foobar")
  and the following fields:

      - address:      The address of the node.
      - dial_address: The dialable address of the node (defaults to address).
      - components:   A list of the components hosted on the node.
      - listeners:    A map from listener names to listener addresses.

  In the example above,

  - The "main" node runs on address "localhost:9000" with dialable address
    "localhost:9000". It hosts the Main component and exports listener
    "lis" on "localhost:8000".
  - The "foobar" node listens on address ":9001" with dialable address
    "localhost:9001". It hosts components Foo and Bar.

  Note that nodes are a logical concept. You can run multiple nodes on the same
  physical machine, or you can run multiple nodes on different machines.

  To deploy an application with "weaver exec", you provide the weaver config
  file, the topology file, and the name of the node to execute. To run the
  example above, we would run the following two commands:

      $ weaver exec weaver.toml topology.toml foobar # run the foobar node
      $ weaver exec weaver.toml topology.toml main   # run the main node
`

// topology describes the topology of a Service Weaver application.
type topology struct {
	DeploymentId string          `toml:"id"`    // unique deployment id
	Nodes        map[string]node `toml:"nodes"` // set of nodes
}

// node describes a collection of components (previously called a colocation
// group). There may be multiple replicas of a node, with each replica running
// a weavelet.
type node struct {
	Address     string            `toml:"address"`      // weavelet address
	DialAddress string            `toml:"dial_address"` // dialable address to all replicas of this node
	Components  []string          `toml:"components"`   // components to run
	Listeners   map[string]string `toml:"listeners"`    // listener addresses, by listener name
}

// parseTopology parses a topology from the provided TOML.
func parseTopology(contents string) (topology, error) {
	var topo topology
	if _, err := toml.Decode(contents, &topo); err != nil {
		return topology{}, err
	}
	return topo, nil
}

// Run runs the "weaver exec" command.
func Run(args []string) {
	flags := flag.NewFlagSet("exec", flag.ExitOnError)
	flags.Usage = func() { fmt.Fprint(os.Stdout, usage) }
	flags.Parse(args)

	if flags.NArg() == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
	if flags.NArg() != 3 {
		fmt.Fprintln(os.Stderr, "usage: weaver exec <config file> <topology file> <node>")
		os.Exit(1)
	}
	if err := runImpl(flags.Arg(0), flags.Arg(1), flags.Arg(2)); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runImpl(configFile, topoFile, nodeName string) error {
	// Parse topology file.
	bytes, err := os.ReadFile(topoFile)
	if err != nil {
		return fmt.Errorf("read %q: %w", topoFile, err)
	}
	topology, err := parseTopology(string(bytes))
	if err != nil {
		return fmt.Errorf("parse %q: %w", topoFile, err)
	}
	node, ok := topology.Nodes[nodeName]
	if !ok {
		return fmt.Errorf("node %q not found in topology file %q", nodeName, topoFile)
	}

	// Collect the diable addresses for every component.
	addresses := map[string][]string{}
	for _, node := range topology.Nodes {
		// TODO(mwhittaker): Right now, we assume tcp addresses. In the future,
		// we may want to support things like "dns://headless-service-name" or
		// "kubelabel://foo".
		var dialAddress string
		if node.DialAddress != "" {
			dialAddress = "tcp://" + node.DialAddress
		} else {
			dialAddress = "tcp://" + node.Address
		}
		for _, component := range node.Components {
			addresses[component] = append(addresses[component], dialAddress)
		}
	}

	// Massage component addresses into resolvers.
	resolvers := map[string]call.Resolver{}
	var errs []error
	for component, addrs := range addresses {
		endpoints := make([]call.Endpoint, len(addrs))
		for i, addr := range addrs {
			endpoint, err := call.ParseNetEndpoint(addr)
			if err != nil {
				err = fmt.Errorf("component %q address %q: %w", component, addr, err)
				errs = append(errs, err)
				continue
			}
			endpoints[i] = endpoint
		}
		resolvers[component] = call.NewConstantResolver(endpoints...)
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}

	// Run the weavelet.
	config := run.Config{
		ConfigFile:      configFile,
		DeploymentId:    topology.DeploymentId,
		Components:      node.Components,
		Resolvers:       resolvers,
		Listeners:       node.Listeners,
		WeaveletAddress: node.Address,
	}
	return run.Run(context.Background(), config)
}
