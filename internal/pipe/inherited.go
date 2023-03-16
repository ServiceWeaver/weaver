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

//go:build !windows

package pipe

import (
	"os"
	"os/exec"
)

// addInheritedFile add a file that will be inherited by the new process.
func addInheritedFile(cmd *exec.Cmd, file *os.File) uintptr {
	// From https://pkg.go.dev/os/exec#Cmd.ExtraFiles: "entry i becomes file
	// descriptor 3+i".
	fd := 3 + len(cmd.ExtraFiles)
	cmd.ExtraFiles = append(cmd.ExtraFiles, file)
	return uintptr(fd)
}
