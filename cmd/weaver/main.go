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
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	itool "github.com/ServiceWeaver/weaver/internal/tool"
	"github.com/ServiceWeaver/weaver/internal/tool/callgraph"
	"github.com/ServiceWeaver/weaver/internal/tool/generate"
	"github.com/ServiceWeaver/weaver/internal/tool/multi"
	"github.com/ServiceWeaver/weaver/internal/tool/single"
	"github.com/ServiceWeaver/weaver/internal/tool/ssh"
	"github.com/ServiceWeaver/weaver/runtime/tool"
)

const usage = `USAGE

  weaver generate                 // weaver code generator
  weaver version                  // show weaver version
  weaver single    <command> ...  // for single process deployments
  weaver multi     <command> ...  // for multiprocess deployments
  weaver ssh       <command> ...  // for multimachine deployments
  weaver gke       <command> ...  // for GKE deployments
  weaver gke-local <command> ...  // for simulated GKE deployments
  weaver kube      <command> ...  // for vanilla Kubernetes deployments

DESCRIPTION

  Use the "weaver" command to deploy and manage Weaver applications.

  The "weaver generate", "weaver version", "weaver single", "weaver multi", and
  "weaver ssh" subcommands are baked in, but all other subcommands of the form
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

	// Handle the internal deployers.
	internals := map[string]map[string]*tool.Command{
		"single": single.Commands,
		"multi":  multi.Commands,
		"ssh":    ssh.Commands,
	}

	switch flag.Arg(0) {
	case "generate":
		generateFlags := flag.NewFlagSet("generate", flag.ExitOnError)
		generateFlags.Usage = func() {
			fmt.Fprintln(os.Stderr, generate.Usage)
		}
		generateFlags.Parse(flag.Args()[1:])
		if err := generate.Generate(".", flag.Args()[1:], generate.Options{}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return

	case "version":
		cmd := itool.VersionCmd("weaver")
		if err := cmd.Fn(context.Background(), flag.Args()[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return

	case "callgraph":
		const usage = `Generate component callgraphs.

Usage:
  weaver callgraph <binary>

Flags:
  -h, --help           Print this help message.

Description:
  "weaver callgraph <file>" outputs a component callgraph in mermaid format
  [1]. These graphs can be included in GitHub README files [2].

[1]: https://mermaid.js.org/
[2]: https://github.blog/2022-02-14-include-diagrams-markdown-files-mermaid/`
		flags := flag.NewFlagSet("callgraph", flag.ExitOnError)
		flags.Usage = func() { fmt.Fprintln(os.Stderr, usage) }
		flags.Parse(flag.Args()[1:])
		if flags.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "ERROR: no binary provided.")
		}
		s, err := callgraph.Mermaid(flags.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		fmt.Println(s)
		return

	case "single", "multi", "ssh":
		os.Args = os.Args[1:]
		tool.Run("weaver "+flag.Arg(0), internals[flag.Arg(0)])
		return

	case "help":
		n := len(flag.Args())
		command := flag.Arg(1)
		switch {
		case n == 1:
			// weaver help
			fmt.Fprint(os.Stdout, usage)
		case n == 2 && command == "generate":
			// weaver help generate
			fmt.Fprintln(os.Stdout, generate.Usage)
		case n == 2 && internals[command] != nil:
			// weaver help <command>
			fmt.Fprintln(os.Stdout, tool.MainHelp("weaver "+command, internals[command]))
		case n == 2:
			// weaver help <external>
			code, err := run(command, []string{"--help"})
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(code)
			}
		case n > 2:
			fmt.Fprintf(os.Stderr, "help: too many arguments. Try 'weaver %s %s --help'\n", command, strings.Join(flag.Args()[2:], " "))
		}
		return
	}

	// Handle all other "weaver <deployer>" subcommands.
	code, err := run(flag.Args()[0], flag.Args()[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(code)
	}
}

// run runs "weaver-<deployer> [arg]..." in a subprocess and returns the
// subprocess' exit code and any error.
func run(deployer string, args []string) (int, error) {
	binary := "weaver-" + deployer
	if _, err := exec.LookPath(binary); err != nil {
		msg := fmt.Sprintf(`"weaver %s" is not a weaver command. See "weaver --help". If you're trying to invoke a custom deployer, the %q binary was not found. You may need to install the %q binary or add it to your PATH.`, deployer, binary, binary)
		return 1, fmt.Errorf(wrap(msg, 80))
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	exitError := &exec.ExitError{}
	if errors.As(err, &exitError) {
		return exitError.ExitCode(), err
	}
	return 1, err
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
