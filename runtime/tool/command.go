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

// Package tool contains utilities for creating Service Weaver tools similar to
// weaver-multi, weaver-gke, and weaver-gke-local.
package tool

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Command is a subcommand of a binary, like the "add" in "git add".
type Command struct {
	Name        string                                // subcommand name
	Description string                                // short one-line description
	Help        string                                // full help message
	Flags       *flag.FlagSet                         // flags
	Fn          func(context.Context, []string) error // passed command line args that occur after the subcommmand name
	Hidden      bool                                  // hide from help messages?
}

// Run executes a collection of subcommands, parsing command line arguments and
// dispatching to the correct subcommand.
func Run(tool string, commands map[string]*Command) {
	// Add a help command.
	if _, ok := commands["help"]; !ok {
		commands["help"] = &Command{
			Name:        "help",
			Description: "Print help for a sub-command",
			Fn: func(_ context.Context, args []string) error {
				if len(args) == 0 {
					fmt.Fprintln(os.Stderr, MainHelp(tool, commands))
					return nil
				}
				if len(args) != 1 {
					return fmt.Errorf("help: too many arguments")
				}
				cmd, ok := commands[args[0]]
				if !ok {
					return fmt.Errorf("help: command %q not found", args[0])
				}
				fmt.Println(commandHelp(cmd))
				return nil
			},
		}
	}

	// Catch -h or --help.
	flags := flag.NewFlagSet(tool, flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, MainHelp(tool, commands))
	}
	if err := flags.Parse(os.Args[1:]); err == flag.ErrHelp {
		os.Exit(0)
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v", tool, err)
		os.Exit(1)
	}

	// Get sub-command.
	cmd, ok := commands[flags.Arg(0)]
	if !ok {
		fmt.Fprintln(os.Stderr, MainHelp(tool, commands))
		os.Exit(1)
	}

	// Parse command flags.
	args := flags.Args()[1:]
	if cmd.Flags != nil {
		cmd.Flags.Usage = func() {
			// Disable Usage here so that cmd.Flags.Parse() won't automatically
			// print the usage before we have a chance to print our command
			// help below (i.e. t.commandHelp(os.Stderr, cmd)).
		}
		if err := cmd.Flags.Parse(args); err != nil {
			fmt.Fprintln(os.Stderr, commandHelp(cmd))
			os.Exit(1)
		}
		args = cmd.Flags.Args()
	}

	// Run command.
	ctx := context.Background()
	if err := cmd.Fn(ctx, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// MainHelp returns the help message for the provided set of commands.
func MainHelp(tool string, commands map[string]*Command) string {
	var sorted []string
	for name, cmd := range commands {
		if !cmd.Hidden {
			sorted = append(sorted, name)
		}
	}
	sort.Strings(sorted)

	var cmds strings.Builder
	for _, name := range sorted {
		cmd := commands[name]
		fmt.Fprintf(&cmds, "  %-12s %s\n", name, cmd.Description)
	}

	return fmt.Sprintf(`Deploy and manage Service Weaver programs.

Usage:
  %s <command> ...

Available Commands:
%s
Flags:
  -h, --help   Print this help message.

Use "%s help <command>" for more information about a command.`, tool, cmds.String(), tool)
}

// commandHelp returns the help message for the provided command.
func commandHelp(cmd *Command) string {
	if cmd.Help == "" {
		return cmd.Description
	}
	return fmt.Sprintf("%s\n\n%s", cmd.Description, cmd.Help)
}

// FlagsHelp pretty prints the set of flags in the provided FlagSet, along with
// their descriptions and default values. Here's an example output:
//
//	-h, --help    Print this help message.
//	--follow      Act like tail -f (default false)
//	--format      Output format (pretty or json) (default pretty)
//	--system      Show system internal logs (default false)
func FlagsHelp(flags *flag.FlagSet) string {
	// This function borrows heavily from flag.PrintDefaults [1] but returns a
	// string instead of printing.
	//
	// TODO(mwhittaker): Have this file call flagsHelp on spec.Flags, rather
	// than having every individual command call it. We could change Command to
	// have Help, Usage, and ExtraHelp fields. This file could then print Help,
	// Usage, flagsHelp, and then ExtraHelp.
	//
	// [1]: https://pkg.go.dev/flag#PrintDefaults
	var b strings.Builder
	flags.VisitAll(func(flag *flag.Flag) {
		if len(flag.Name) == 1 {
			fmt.Fprintf(&b, "  -%s", flag.Name)
		} else {
			fmt.Fprintf(&b, "  --%s", flag.Name)
		}
		fmt.Fprintf(&b, "\t%s (default %s)\n", flag.Usage, flag.DefValue)
	})
	return strings.TrimSuffix(b.String(), "\n")
}
