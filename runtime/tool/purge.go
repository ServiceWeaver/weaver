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
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

// PurgeSpec configures the command returned by PurgeCmd.
type PurgeSpec struct {
	Tool  string   // tool name (e.g., "weaver multi")
	Kill  string   // regex of processes to kill, or empty
	Paths []string // paths to delete

	force bool // the --force flag
}

// PurgeCmd returns a command to delete a set of paths.
func PurgeCmd(spec *PurgeSpec) *Command {
	// Create flags and help.
	flags := flag.NewFlagSet("purge", flag.ContinueOnError)
	flags.BoolVar(&spec.force, "force", false, "Purge without prompt")
	const help = `Usage:
  {{.Tool}} purge [--force]

Flags:
  -h, --help	Print this help message.
{{.Flags}}

Description:
  "{{.Tool}} purge" kills all "{{.Tool}}"-related processes and deletes any logs
  and data produced by "{{.Tool}}".`
	var b strings.Builder
	t := template.Must(template.New(spec.Tool).Parse(help))
	content := struct{ Tool, Flags string }{spec.Tool, FlagsHelp(flags)}
	if err := t.Execute(&b, content); err != nil {
		panic(err)
	}

	return &Command{
		Name:        "purge",
		Flags:       flags,
		Description: "Purge processes and data",
		Help:        b.String(),
		Fn:          spec.purge,
	}
}

func (spec *PurgeSpec) purge(context.Context, []string) error {
	if !spec.force {
		// Gather the set of processes to kill.
		tokill := ""
		if spec.Kill != "" {
			var err error
			tokill, err = pgrep(spec.Kill)
			if err != nil {
				return err
			}
			if tokill == "" {
				tokill = "    (no matching processes found)\n"
			}
		}

		// Warn the user they're about to delete stuff.
		//
		// TODO(mwhittaker): If spec.Kill is empty, don't print out a message
		// saying we're going to kill something.
		var paths strings.Builder
		for _, path := range spec.Paths {
			fmt.Fprintf(&paths, "    - %s\n", path)
		}

		fmt.Printf(`WARNING: You are about to kill all processes which match the following regex:

    %s

This currently includes the following processes:

%s
You will also delete the following paths used to store logs and data for
%q Service Weaver applications. This data will be deleted
immediately and irrevocably. Are you sure you want to proceed?"

%s
Enter (y)es to continue: `, spec.Kill, indent(tokill, 4), spec.Tool, paths.String())

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

	// Kill the processes.
	if spec.Kill != "" {
		killed, err := pkill(spec.Kill)
		if err != nil {
			return err
		}
		fmt.Print(killed)
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
	return nil
}

// pgrep returns the output of 'pgrep -a -f <regex>'.
func pgrep(regex string) (string, error) {
	// "-a" causes pgrep to output the full command line of matched processes.
	// "-f" causes the regex to match the full command line.
	cmd := exec.Command("pgrep", "-a", "-f", regex)
	out, err := cmd.Output()

	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		// pgrep's man page explains that an exit code of 1 indicates that
		// "no processes matched". We don't treat this as an error.
		return "", nil
	case errors.As(err, &exit):
		return "", fmt.Errorf("%w: %s", err, string(exit.Stderr))
	case err != nil:
		return "", err
	default:
		return string(out), nil
	}
}

// pkill returns the output of 'pkill -f <regex>'.
func pkill(regex string) (string, error) {
	// "-f" causes the regex to match the full command line.
	cmd := exec.Command("pkill", "-f", regex)
	out, err := cmd.Output()

	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		// pkill's man page explains that an exit code of 1 indicates that "no
		// processes matched or none of them could be signalled". We'll assume
		// that no processes were matched, as that is far more likely than them
		// failing to get signalled.
		return "", nil
	case errors.As(err, &exit):
		return "", fmt.Errorf("%w: %s", err, string(exit.Stderr))
	case err != nil:
		return "", err
	default:
		return string(out), nil
	}
}

// indent indents the provided string n spaces.
func indent(s string, n int) string {
	tab := strings.Repeat(" ", n)
	return tab + strings.ReplaceAll(s, "\n", "\n"+tab)
}
