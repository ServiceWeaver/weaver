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

package status

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	protos "github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/tool"
	pprof "github.com/google/pprof/profile"
)

// TODO(mwhittaker): Right now, a user has to (1) run `weaver profile` to get a
// profile and then (2) pass it to the `go pprof` command. Srdjan suggested we
// have `weaver profile` wrap `pprof` to make this a single step instead of two.

var (
	profileFlags    = flag.NewFlagSet("profile", flag.ContinueOnError)
	profileDuration = profileFlags.Duration("duration", 30*time.Second, "Duration of cpu profiles")
	profileType     = profileFlags.String("type", "cpu", `Profile type; "cpu" or "heap"`)
)

// ProfileCommand returns a "profile" subcommand that gathers pprof profiles.
func ProfileCommand(toolName string, registry func(context.Context) (*Registry, error)) *tool.Command {
	const help = `Usage:
  {{.Tool}} profile [options] <deployment>

Flags:
  -h, --help	Print this help message.
{{.Flags}}

Description:
  '{{.Tool}} profile <deployment>' profiles a deployed application and
  fetches its profile to the local machine. <deployment> is the id of the
  deployment which can be found using '{{.Tool}} status' or '{{.Tool}}
  dashboard'. The id is also printed when the deployment starts. '{{.Tool}}
  profile' writes the profile to a file that can be passed to pprof.

Deployment Ids:
  For convenience, '{{.Tool}} profile' accepts a uniquely identifying prefix
  of a deployment id. For example, consider deployments with the following ids:

    - 2c80d811-6120-4881-8e81-7943a61a31d7
    - 2c57690a-40b9-4a9a-90b2-a887f40df4e0

  The prefix "2c8" uniquely identifies the first deployment, but the prefixes
  "2" and "2c" are ambiguous.

Examples:
  TODO(rgrandl): Show how to take a profile and pass it to pprof.`
	var b strings.Builder
	t := template.Must(template.New(toolName).Parse(help))
	content := struct{ Tool, Flags string }{toolName, tool.FlagsHelp(profileFlags)}
	if err := t.Execute(&b, content); err != nil {
		panic(err)
	}

	return &tool.Command{
		Name:        "profile",
		Description: "Profile a running Service Weaver application",
		Help:        b.String(),
		Flags:       profileFlags,
		Fn: func(ctx context.Context, args []string) error {
			// Validate command line arguments.
			if len(args) != 1 {
				return fmt.Errorf("usage: %s profile [options] <deployment>", toolName)
			}
			prefix := args[0]
			if prefix == "" {
				return fmt.Errorf("usage: %s profile [options] <deployment>", toolName)
			}
			if *profileType != "cpu" && *profileType != "heap" {
				return fmt.Errorf("invalid profile type %q; want %q or %q", *profileType, "cpu", "heap")
			}

			// Get the corresponding deployment id.
			registry, err := registry(ctx)
			if err != nil {
				return fmt.Errorf("create registry: %w", err)
			}
			regs, err := registry.List(ctx)
			if err != nil {
				return fmt.Errorf("get registrations: %w", err)
			}
			var candidates []Registration
			for _, reg := range regs {
				if strings.HasPrefix(reg.DeploymentId, prefix) {
					candidates = append(candidates, reg)
				}
			}
			if len(candidates) == 0 {
				return fmt.Errorf("no deployment with prefix %q found", prefix)
			}
			if len(candidates) > 1 {
				fmt.Fprintf(os.Stderr, "The deployment id prefix %q is ambiguous. Expand the prefix to identify one of the following deployments:\n", prefix)
				for _, candidate := range candidates {
					fmt.Fprintf(os.Stderr, "  - %s\n", candidate.DeploymentId)
				}
				return fmt.Errorf("multiple deployments with prefix %q found", prefix)
			}
			client := NewClient(candidates[0].Addr) // candidates[0] is the only candidate

			// Get the deployment's status.
			status, err := client.Status(ctx)
			if err != nil {
				return err
			}

			// Form the profile request.
			req := &protos.GetProfileRequest{}
			if *profileType == "heap" {
				req.ProfileType = protos.ProfileType_Heap
			} else {
				req.ProfileType = protos.ProfileType_CPU
				req.CpuDurationNs = profileDuration.Nanoseconds()
			}

			// Start the profile request.
			var reply *protos.GetProfileReply
			done := make(chan struct{})
			go func() {
				defer close(done)
				reply, err = client.Profile(ctx, req)
			}()

			// Wait for the profile to finish. If the profile is going to take a long
			// time, show a spinner to the user, so they know how long to wait.
			if *profileType == "cpu" && *profileDuration > time.Second {
				spinner("Profiling in progress...", *profileDuration, done)
			} else {
				<-done
			}

			if reply == nil {
				return fmt.Errorf("nil profile: %v", err)
			}
			if len(reply.Data) == 0 && err != nil {
				return fmt.Errorf("cannot create profile: %w", err)
			}
			if len(reply.Data) == 0 {
				return fmt.Errorf("empty profile data")
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "Partial profile data received: the profile may not be accurate. Errors:", err)
			}
			prof, err := pprof.ParseData(reply.Data)
			if err != nil {
				return fmt.Errorf("error parsing profile: %w", err)
			}

			// Save the profile in a file.
			name := fmt.Sprintf("serviceweaver_%s_%s_profile_*.pb.gz", status.App, *profileType)
			f, err := os.CreateTemp(os.TempDir(), name)
			if err != nil {
				return fmt.Errorf("saving profile: %w", err)
			}
			name = f.Name()
			if err := prof.Write(f); err != nil {
				f.Close()
				os.Remove(name)
				return fmt.Errorf("writing profile: %w", err)
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("writing profile: %w", err)
			}
			fmt.Println(name)
			return nil
		},
	}
}

// spinner prints a spinning progress bar to stderr:
//
//	⠏ [8s/10s] Profiling in progress...
//
// The spinner prints the provided message until done is is closed. The
// provided duration is an estimate of when done should be closed. The idea for
// this spinner was taken from [1].
//
// [1]: https://github.com/sindresorhus/cli-spinners
func spinner(msg string, duration time.Duration, done chan struct{}) {
	start := time.Now()
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			fmt.Fprintln(os.Stderr)
			return

		case <-ticker.C:
			since := time.Since(start).Truncate(time.Second)
			fmt.Fprintf(os.Stderr, "%s [%s/%s] %s\r", frames[i], since, duration, msg)
			i = (i + 1) % len(frames)
		}
	}
}
