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
	"strconv"

	"github.com/ServiceWeaver/weaver/internal/pipe"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// Child manages the child of an envelope. This is typically a child process, but
// tests may manage some in-process resources.
type Child interface {
	// Start starts the child.
	// REQUIRES: Start, Wait have not been called.
	Start(context.Context, *protos.AppConfig) error

	// Wait for the child to exit.
	// REQUIRES: Start has been called.
	// REQUIRES: Wait has not been called.
	Wait() error

	// Different IO streams connecting us to the child
	Reader() io.ReadCloser  // Can be used to receive messages from the Child
	Writer() io.WriteCloser // Can be used to send messages to the Child
	Stdout() io.ReadCloser  // Delivers Child stdout
	Stderr() io.ReadCloser  // Delivers Child stderr

	// Identifier for child process, if available.
	Pid() (int, bool)
}

// ProcessChild is a Child implemented as a process.
type ProcessChild struct {
	cmd    *pipe.Cmd // command that started the weavelet
	reader io.ReadCloser
	writer io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

var _ Child = &ProcessChild{}

func (p *ProcessChild) Reader() io.ReadCloser  { return p.reader }
func (p *ProcessChild) Writer() io.WriteCloser { return p.writer }
func (p *ProcessChild) Stdout() io.ReadCloser  { return p.stdout }
func (p *ProcessChild) Stderr() io.ReadCloser  { return p.stderr }
func (p *ProcessChild) Pid() (int, bool)       { return p.cmd.Process.Pid, true }

func (p *ProcessChild) Start(ctx context.Context, config *protos.AppConfig) error {
	cmd := pipe.CommandContext(ctx, config.Binary, config.Args...)

	// Create the request/response pipes first, so we can fill cmd.Env and detect
	// any errors early.
	pipePair, err := cmd.MakePipePair()
	if err != nil {
		return fmt.Errorf("create request/response pipes: %w", err)
	}

	// Create pipes that capture child outputs.
	outpipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	errpipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	// Setup environment for child process.
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", runtime.ToWeaveletKey, strconv.FormatUint(uint64(pipePair.ChildReader), 10)))
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", runtime.ToEnvelopeKey, strconv.FormatUint(uint64(pipePair.ChildWriter), 10)))
	cmd.Env = append(cmd.Env, config.Env...)

	if err := cmd.Start(); err != nil {
		return err
	}

	p.cmd = cmd
	p.reader = pipePair.ParentReader
	p.writer = pipePair.ParentWriter
	p.stdout = outpipe
	p.stderr = errpipe
	return nil
}

func (p *ProcessChild) Wait() error {
	err := p.cmd.Wait()
	p.cmd.Cleanup()
	return err
}

// inProcessChild is a fake envelope.Child that represents the in-process weavelet.
type inProcessChild struct {
	ctx    context.Context
	reader io.ReadCloser
	writer io.WriteCloser
}

var _ Child = &inProcessChild{}

func NewInProcessChild(reader io.ReadCloser, writer io.WriteCloser) Child {
	return &inProcessChild{
		reader: reader,
		writer: writer,
	}
}

func (p *inProcessChild) Reader() io.ReadCloser  { return p.reader }
func (p *inProcessChild) Writer() io.WriteCloser { return p.writer }
func (p *inProcessChild) Stdout() io.ReadCloser  { return nil }
func (p *inProcessChild) Stderr() io.ReadCloser  { return nil }
func (p *inProcessChild) Pid() (int, bool)       { return 0, false }

func (p *inProcessChild) Start(ctx context.Context, config *protos.AppConfig) error {
	p.ctx = ctx
	return nil
}

func (p *inProcessChild) Wait() error {
	<-p.ctx.Done()
	return nil
}
