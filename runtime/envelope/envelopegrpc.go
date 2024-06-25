// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package envelope

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/controlgrpc"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	// We rely on the weaver.controller component registrattion entry.
	_ "github.com/ServiceWeaver/weaver"
)

// EnvelopeGrpc starts and manages a weavelet in a subprocess.
type EnvelopeGrpc struct {
	controlgrpc.DeployerControlGrpcServer
	weavelet *weaveletInfo

	// Fields below are constant after construction.
	ctx       context.Context
	ctxCancel context.CancelFunc
	config    *protos.AppConfig
	args      *protos.WeaveletArgs

	deployerAddr net.Listener
}

type weaveletInfo struct {
	addr       string
	child      Child                                 // weavelet process handle
	controller controlgrpc.WeaveletControlGrpcClient // Stub that talks to the weavelet grpc controller
	conn       *grpc.ClientConn                      // grpc client conn to the weavelet control
}

func NewEnvelopeGrpc(ctx context.Context, wlet *protos.WeaveletArgs, config *protos.AppConfig) (*EnvelopeGrpc, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() { cancel() }() // cancel may be changed below if we want to delay it

	ds, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}

	e := &EnvelopeGrpc{
		ctx:          ctx,
		ctxCancel:    cancel,
		args:         protomsg.Clone(wlet),
		config:       config,
		deployerAddr: ds,
	}
	e.args.DeployerSocket = ds.Addr().String()

	// Start the weavelet.
	child := &ProcessChild{}
	if err := child.Start(ctx, e.config, e.args); err != nil {
		return nil, fmt.Errorf("NewEnvelope: %w", err)
	}

	// Create a grpc client to the weavelet control component.
	conn, err := grpc.Dial(wlet.ControlSocket, grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	controller := controlgrpc.NewWeaveletControlGrpcClient(conn)

	// Send an init weavelet to the weavelet.
	reply, err := controller.InitWeavelet(e.ctx, &protos.InitWeaveletRequest{
		Sections: config.Sections,
	})
	if err != nil {
		return nil, err
	}

	e.weavelet = &weaveletInfo{
		addr:       reply.DialAddr,
		child:      child,
		controller: controller,
		conn:       conn,
	}

	cancel = func() {} // Delay real context cancellation
	return e, nil
}

// WeaveletControl returns the controller component for the weavelet managed by this envelope.
func (e *EnvelopeGrpc) WeaveletControl() controlgrpc.WeaveletControlGrpcClient {
	return e.weavelet.controller
}

// Serve accepts incoming messages from the weavelet.
func (e *EnvelopeGrpc) Serve(h controlgrpc.DeployerControlGrpcServer) error {
	/*	deployerAddr, err := net.Listen("tcp", e.args.DeployerSocket)
		if err != nil {
			return err
		}*/

	var running errgroup.Group

	var stopErr error
	var once sync.Once
	stop := func(err error) {
		once.Do(func() {
			stopErr = err
		})
		e.ctxCancel()
		e.weavelet.conn.Close()
	}

	// Capture stdout and stderr from the weavelet.
	if stdout := e.weavelet.child.Stdout(); stdout != nil {
		running.Go(func() error {
			err := e.logLines("stdout", stdout, h)
			stop(err)
			return err
		})
	}
	if stderr := e.weavelet.child.Stderr(); stderr != nil {
		running.Go(func() error {
			err := e.logLines("stderr", stderr, h)
			stop(err)
			return err
		})
	}

	// Start the goroutine watching the context for cancellation.
	running.Go(func() error {
		<-e.ctx.Done()
		err := e.ctx.Err()
		stop(err)
		return err
	})

	// Start the goroutine to handle deployer control calls.
	running.Go(func() error {
		s := grpc.NewServer()
		defer s.GracefulStop()
		controlgrpc.RegisterDeployerControlGrpcServer(s, h)
		err := s.Serve(e.deployerAddr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to serve deployer control component: %v\n", err)
		}
		stop(err)
		return err
	})

	running.Wait()

	// Wait for the weavelet command to finish. This needs to be done after
	// we're done reading from stdout/stderr pipes, per comments on
	// exec.Cmd.StdoutPipe and exec.Cmd.StderrPipe.
	stop(e.weavelet.child.Wait())

	return stopErr
}

// Pid returns the process id of the weavelet, if it is running in a separate process.
func (e *EnvelopeGrpc) Pid() (int, bool) {
	return e.weavelet.child.Pid()
}

// WeaveletAddress returns the address that other components should dial to communicate with the
// weavelet.
func (e *EnvelopeGrpc) WeaveletAddress() string {
	return e.weavelet.addr
}

// UpdateComponents updates the weavelet with the latest set of components it
// should be running.
func (e *EnvelopeGrpc) UpdateComponents(components []string) error {
	req := &protos.UpdateComponentsRequest{
		Components: components,
	}
	_, err := e.weavelet.controller.UpdateComponents(context.TODO(), req)
	return err
}

// UpdateRoutingInfo updates the weavelet with a component's most recent
// routing info.
func (e *EnvelopeGrpc) UpdateRoutingInfo(routing *protos.RoutingInfo) error {
	req := &protos.UpdateRoutingInfoRequest{
		RoutingInfo: routing,
	}
	_, err := e.weavelet.controller.UpdateRoutingInfo(context.TODO(), req)
	return err
}

func (e *EnvelopeGrpc) logLines(component string, src io.Reader, h controlgrpc.DeployerControlGrpcServer) error {
	// Fill partial log entry.
	entry := &protos.LogEntry{
		App:       e.args.App,
		Version:   e.args.DeploymentId,
		Component: component,
		Node:      e.args.Id,
		Level:     component, // Either "stdout" or "stderr"
		File:      "",
		Line:      -1,
	}
	batch := &protos.LogEntryBatch{}
	batch.Entries = append(batch.Entries, entry)

	rdr := bufio.NewReader(src)
	for {
		line, err := rdr.ReadBytes('\n')
		// Note: both line and err may be present.
		if len(line) > 0 {
			entry.Msg = string(dropNewline(line))
			entry.TimeMicros = 0 // In case previous LogBatch mutated it
			if _, err := h.LogBatch(e.ctx, batch); err != nil {
				return err
			}
		}
		if err != nil {
			return fmt.Errorf("capture %s: %w", component, err)
		}
	}
}
