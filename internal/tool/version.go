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

	"github.com/ServiceWeaver/weaver/runtime/tool"
)

// VersionCmd returns a command to show a deployer's version.
func VersionCmd(toolname string) *tool.Command {
	return &tool.Command{
		Name:        "version",
		Flags:       flag.NewFlagSet("version", flag.ContinueOnError),
		Description: fmt.Sprintf("Show %q version", toolname),
		Help:        fmt.Sprintf("Usage:\n  %s version", toolname),
		Fn: func(context.Context, []string) error {
			v, err := SelfVersion()
			if err != nil {
				return err
			}
			fmt.Printf("%s %s %s/%s\n", toolname, v, runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}

// SelfVersion returns the version of the running tool binary.
func SelfVersion() (string, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		// Should never happen.
		return "", fmt.Errorf("tool binary must be built from a module")
	}
	return info.Main.Version, nil
}
