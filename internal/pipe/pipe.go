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

// Package pipe extends os.exec, making it easier to create pipes to subcommands.
package pipe

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Cmd is drop-in replacement for exec.Cmd, extended with the Pipe method.
type Cmd struct {
	*exec.Cmd
	closeAfterStart []io.Closer // closed after Start()
	closeAfterWait  []io.Closer // closed after Wait()
}

// CommandContext returns a new Cmd. See exec.CommandContext for details.
func CommandContext(ctx context.Context, name string, arg ...string) *Cmd {
	return &Cmd{Cmd: exec.CommandContext(ctx, name, arg...)}
}

// PipePair holds a pair of pipes that can be used for bi-directional
// communication with a child process.
type PipePair struct {
	ParentReader io.ReadCloser  // Reader from which parent can read
	ParentWriter io.WriteCloser // Writer to which parent can write
	ChildReader  uintptr        // Descriptor from which child can read
	ChildWriter  uintptr        // Descriptor to which child can write
}

// MakePipePair makes a pair of pipes that can be used for bi-directional
// communication with the child process.
//
// Cmd.ExtraFiles should not be modified directly if MakePipePair is called.
//
// Wait will close ParentWriter automatically after seeing the command exit. A
// caller need only close ParentWriter to force the pipe to close sooner. For
// example, if the command being run will not exit until standard input is
// closed, the caller must close ParentWriter.
//
// Wait will close ParentReader automatically after seeing the command exit, so
// most callers need not close ParentReader themselves. It is thus incorrect to
// call Wait before all reads from ParentReader have completed. For the same
// reason, it is incorrect to use Run when using MakePipePair. See the
// exec.Cmd.StdoutPipe example [1] for idiomatic usage.
//
// [1]: https://pkg.go.dev/os/exec#example-Cmd.StdoutPipe
func (c *Cmd) MakePipePair() (PipePair, error) {
	if c.Process != nil {
		return PipePair{}, fmt.Errorf("capture: MakePipePair after process started")
	}

	r1, w1, err := os.Pipe()
	if err != nil {
		return PipePair{}, err
	}
	r2, w2, err := os.Pipe()
	if err != nil {
		r1.Close()
		w1.Close()
		return PipePair{}, err
	}
	return PipePair{
		ChildReader:  c.registerPipe(w1, r1),
		ChildWriter:  c.registerPipe(r2, w2),
		ParentWriter: w1,
		ParentReader: r2,
	}, nil
}

func (c *Cmd) registerPipe(local, remote *os.File) uintptr {
	c.closeAfterStart = append(c.closeAfterStart, remote)
	c.closeAfterWait = append(c.closeAfterWait, local)
	return addInheritedFile(c.Cmd, remote)
}

// Start is identical to exec.Command.Start.
func (c *Cmd) Start() error {
	if err := c.Cmd.Start(); err != nil {
		return err
	}
	closeAll(&c.closeAfterStart)
	return nil
}

// Wait is identical to exec.Command.Wait.
func (c *Cmd) Wait() error {
	if err := c.Cmd.Wait(); err != nil {
		return err
	}
	closeAll(&c.closeAfterWait)
	return nil
}

// Cleanup cleans up any unused resources.
func (c *Cmd) Cleanup() {
	closeAll(&c.closeAfterStart)
	closeAll(&c.closeAfterWait)
}

func closeAll(files *[]io.Closer) {
	for _, f := range *files {
		f.Close()
	}
	*files = nil
}
