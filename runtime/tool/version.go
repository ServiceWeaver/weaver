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
	"context"
	"flag"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/version"
)

// VersionCmd returns a command to show a deployer's version.
func VersionCmd(tool string) *Command {
	return &Command{
		Name:        "version",
		Flags:       flag.NewFlagSet("version", flag.ContinueOnError),
		Description: fmt.Sprintf("Show %q version", tool),
		Help:        fmt.Sprintf("Usage:\n  %s version", tool),
		Fn: func(context.Context, []string) error {
			deployerAPI := fmt.Sprintf("%d.%d.%d", version.Major, version.Minor, version.Patch)
			codegenAPI := fmt.Sprintf("%d.%d.0", codegen.Major, codegen.Minor)
			release := "?"
			commit := "?"
			if info, ok := debug.ReadBuildInfo(); ok {
				release = info.Main.Version
				for _, setting := range info.Settings {
					// vcs.revision stores the commit at which the weaver tool
					// was built. See [1] for more information.
					//
					// [1]: https://pkg.go.dev/runtime/debug#BuildSetting
					if setting.Key == "vcs.revision" {
						commit = setting.Value
						break
					}
				}
			}
			fmt.Printf("%s %s\n", tool, release)
			fmt.Printf("target: %s/%s\n", runtime.GOOS, runtime.GOARCH)
			fmt.Printf("commit: %s\n", commit)
			fmt.Printf("deployer API: %s\n", deployerAPI)
			fmt.Printf("codegen API: %s\n", codegenAPI)
			return nil
		},
	}
}
