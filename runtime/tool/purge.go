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

package tool

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/template"
)

// PurgeSpec configures the command returned by PurgeCmd.
type PurgeSpec struct {
	Tool  string   // tool name (e.g., "weaver multi")
	Paths []string // paths to delete
	// TODO(mwhittaker): Add a regex to kill processes.

	force bool // the --force flag
}

// PurgeCmd returns a command to delete a set of paths.
func PurgeCmd(spec *PurgeSpec) *Command {
	// Sanity check spec.
	if len(spec.Paths) == 0 {
		panic(fmt.Errorf("PurgeSpec with no paths"))
	}

	// Create flags and help.
	flags := flag.NewFlagSet("purge", flag.ContinueOnError)
	flags.BoolVar(&spec.force, "force", false, "Purge without prompt")
	const help = `Usage:
  {{.Tool}} purge [--force]

Flags:
  -h, --help	Print this help message.
{{.Flags}}

Description:
  "{{.Tool}} purge" deletes any logs or data produced by {{.Tool}}.`
	var b strings.Builder
	t := template.Must(template.New(spec.Tool).Parse(help))
	content := struct{ Tool, Flags string }{spec.Tool, FlagsHelp(flags)}
	if err := t.Execute(&b, content); err != nil {
		panic(err)
	}

	return &Command{
		Name:        "purge",
		Flags:       flags,
		Description: "Purge Service Weaver logs and data",
		Help:        b.String(),
		Fn:          spec.purge,
	}
}

func (spec *PurgeSpec) purge(context.Context, []string) error {
	if !spec.force {
		// Warn the user they're about to delete stuff.
		var b strings.Builder
		for _, path := range spec.Paths {
			fmt.Fprintf(&b, "    - %s\n", path)
		}
		fmt.Printf(`WARNING: You are about to delete the following directories used to store logs
and data for %q Service Weaver applications. This data will be deleted
immediately and irrevocably. Any currently running applications deployed with
%q will be corrupted. Are you sure you want to proceed?"

%s
Enter (y)es to continue: `, spec.Tool, spec.Tool, b.String())

		// Get confirmation from the user.
		reader := bufio.NewReader(os.Stdin)
		text, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		text = text[:len(text)-1] // strip the trailing "\n"
		text = strings.ToLower(text)
		if !(text == "y" || text == "yes") {
			fmt.Println("")
			fmt.Println("Purge aborted.")
			return nil
		}
		fmt.Println("")
	}

	// Delete the directories.
	for _, path := range spec.Paths {
		fmt.Printf("Deleting %s... ", path)
		if err := os.RemoveAll(path); err != nil {
			fmt.Println("❌")
			return err
		}
		fmt.Println("✅")
	}

	// TODO(mwhittaker): Delete lingering processes too?
	return nil
}
