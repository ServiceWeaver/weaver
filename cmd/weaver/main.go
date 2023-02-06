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

// Weaver deploys and manages Weaver applications. Run "weaver -help" for
// more information.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ServiceWeaver/weaver/internal/tool/generate"
	"github.com/ServiceWeaver/weaver/internal/tool/multi"
	"github.com/ServiceWeaver/weaver/internal/tool/single"
	"github.com/ServiceWeaver/weaver/internal/tool/ssh"
	"github.com/ServiceWeaver/weaver/runtime/tool"
)

const usage = `USAGE

  weaver generate                 // weaver code generator
  weaver single    <command> ...  // for single process deployments
  weaver multi     <command> ...  // for multiprocess deployments
  weaver ssh       <command> ...  // for multimachine deployments
  weaver gke       <command> ...  // for GKE deployments
  weaver gke-local <command> ...  // for simulated GKE deployments

DESCRIPTION

  Use the "weaver" command to deploy and manage Weaver applications.

  The "weaver generate", "weaver single", "weaver multi", and "weaver ssh"
  subcommands are built-in, but all other subcommands of the form
  "weaver <deployer>" dispatch to a binary called "weaver-<deployer>".
  "weaver gke status", for example, dispatches to "weaver-gke status".
`

func main() {
	// Parse flags.
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	if len(flag.Args()) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	// Handle the built-in subcommands.
	switch flag.Args()[0] {
	case "generate":
		generateFlags := flag.NewFlagSet("generate", flag.ExitOnError)
		generateFlags.Usage = func() {
			fmt.Fprintln(os.Stderr, generate.Usage)
		}
		generateFlags.Parse(flag.Args()[1:])
		if err := generate.Generate(".", flag.Args()[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	case "single":
		os.Args = os.Args[1:]
		tool.Run("weaver single", single.Commands)
		return
	case "multi":
		os.Args = os.Args[1:]
		tool.Run("weaver multi", multi.Commands)
		return
	case "ssh":
		os.Args = os.Args[1:]
		tool.Run("weaver ssh", ssh.Commands)
		return
	}

	// Handle all other "weaver <deployer>" subcommands.
	binary := "weaver-" + flag.Args()[0]
	if _, err := exec.LookPath(binary); err != nil {
		msg := fmt.Sprintf(`"weaver %s" is not a weaver command. See "weaver --help". If you're trying to invoke a custom deployer, the %q binary was not found. You may need to install the %q binary or add it to your PATH.`, flag.Args()[0], binary, binary)
		fmt.Fprintln(os.Stderr, wrap(msg, 80))
		os.Exit(1)
	}
	cmd := exec.Command(binary, flag.Args()[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		return
	}
	exitError := &exec.ExitError{}
	if errors.As(err, &exitError) {
		os.Exit(exitError.ExitCode())
	}
	os.Exit(1)
}

// wrap trims whitespace in the provided string and wraps it to n characters.
func wrap(s string, n int) string {
	var b strings.Builder
	k := 0
	for i, word := range strings.Fields(s) {
		if i == 0 {
			k = len(word)
			fmt.Fprintf(&b, "%s", word)
		} else if k+len(word)+1 > n {
			k = len(word)
			fmt.Fprintf(&b, "\n%s", word)
		} else {
			k += len(word) + 1
			fmt.Fprintf(&b, " %s", word)
		}
	}
	return b.String()
}
