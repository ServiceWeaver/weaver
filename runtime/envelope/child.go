// Copyright 2024 Google LLC
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

package envelope

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ServiceWeaver/weaver/internal/pipe"
	"github.com/ServiceWeaver/weaver/internal/proto"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// Child manages the child of an envelope. This is typically a child process, but
// tests may manage some in-process resources.
type Child interface {
	// Start starts the child.
	// REQUIRES: Start, Wait have not been called.
	Start(context.Context, *protos.AppConfig, *protos.WeaveletArgs) error

	// Wait for the child to exit.
	// REQUIRES: Start has been called.
	// REQUIRES: Wait has not been called.
	Wait() error

	// Different IO streams connecting us to the child
	Stdout() io.ReadCloser // Delivers Child stdout
	Stderr() io.ReadCloser // Delivers Child stderr

	// Identifier for child process, if available.
	Pid() (int, bool)
}

// ProcessChild is a Child implemented as a process.
type ProcessChild struct {
	cmd    *pipe.Cmd // command that started the weavelet
	stdout io.ReadCloser
	stderr io.ReadCloser
}

var _ Child = &ProcessChild{}

func (p *ProcessChild) Stdout() io.ReadCloser { return p.stdout }
func (p *ProcessChild) Stderr() io.ReadCloser { return p.stderr }
func (p *ProcessChild) Pid() (int, bool)      { return p.cmd.Process.Pid, true }

func (p *ProcessChild) Start(ctx context.Context, config *protos.AppConfig, args *protos.WeaveletArgs) error {
	argsEnv, err := proto.ToEnv(args)
	if err != nil {
		return fmt.Errorf("encoding weavelet start message: %w", err)
	}

	cmd := pipe.CommandContext(ctx, config.Binary, config.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Create pipes that capture child outputs.
	//	outpipe, err := cmd.StdoutPipe()
	//	if err != nil {
	//		return fmt.Errorf("create stdout pipe: %w", err)
	//	}
	//	errpipe, err := cmd.StderrPipe()
	//	if err != nil {
	//		return fmt.Errorf("create stderr pipe: %w", err)
	//	}

	// Setup environment for child process.
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", runtime.WeaveletArgsKey, argsEnv))
	cmd.Env = append(cmd.Env, config.Env...)

	if err := cmd.Start(); err != nil {
		return err
	}

	p.cmd = cmd
	//	p.stdout = outpipe
	//	p.stderr = errpipe
	return nil
}

func (p *ProcessChild) Wait() error {
	err := p.cmd.Wait()
	p.cmd.Cleanup()
	return err
}

// InProcessChild is a fake envelope.Child that represents the in-process weavelet.
type InProcessChild struct {
	ctx     context.Context
	args    *protos.WeaveletArgs
	started chan struct{}
}

var _ Child = &InProcessChild{}

func NewInProcessChild() *InProcessChild {
	return &InProcessChild{
		started: make(chan struct{}),
	}
}

func (p *InProcessChild) Stdout() io.ReadCloser { return nil }
func (p *InProcessChild) Stderr() io.ReadCloser { return nil }
func (p *InProcessChild) Pid() (int, bool)      { return 0, false }

func (p *InProcessChild) Start(ctx context.Context, config *protos.AppConfig, args *protos.WeaveletArgs) error {
	p.ctx = ctx
	p.args = protomsg.Clone(args)
	close(p.started)
	return nil
}

func (p *InProcessChild) Wait() error {
	<-p.ctx.Done()
	return nil
}

func (p *InProcessChild) Args() *protos.WeaveletArgs {
	<-p.started
	return p.args
}
