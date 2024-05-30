package envelope

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/control"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	// We rely on the weaver.controller component registrattion entry.
	_ "github.com/ServiceWeaver/weaver"
)

// EnvelopeGrpc starts and manages a weavelet in a subprocess.
//
// For more information, refer to runtime/protos/runtime.proto and
// https://serviceweaver.dev/blog/deployers.html.
type EnvelopeGrpc struct {
	control.DeployerControlGrpcServer

	// Fields below are constant after construction.
	ctx          context.Context
	ctxCancel    context.CancelFunc
	weavelet     *protos.WeaveletArgs
	weaveletAddr string
	config       *protos.AppConfig
	child        Child                             // weavelet process handle
	controller   control.WeaveletControlGrpcClient // Stub that talks to the weavelet grpc controller
}

func NewEnvelopeGrpc(ctx context.Context, wlet *protos.WeaveletArgs, config *protos.AppConfig, options Options) (*EnvelopeGrpc, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() { cancel() }() // cancel may be changed below if we want to delay it

	e := &EnvelopeGrpc{
		ctx:       ctx,
		ctxCancel: cancel,
		weavelet:  protomsg.Clone(wlet),
		config:    config,
	}

	child := &ProcessChild{}
	if err := child.Start(ctx, e.config, e.weavelet); err != nil {
		return nil, fmt.Errorf("NewEnvelope: %w", err)
	}

	weaveletCtrlFn := func() (control.WeaveletControlGrpcClient, error) {
		conn, err := grpc.Dial(wlet.ControlSocket, grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		// Robert - close connection somehow!? conn.Close()
		return control.NewWeaveletControlGrpcClient(conn), nil
	}

	controller, err := weaveletCtrlFn()
	if err != nil {
		return nil, fmt.Errorf("oh nap, err to get controller %v", err)
	}
	e.controller = controller

	reply, err := controller.InitWeavelet(e.ctx, &protos.InitWeaveletRequest{
		Sections: config.Sections,
	})
	if err != nil {
		return nil, err
	}
	e.weaveletAddr = reply.DialAddr

	e.child = child

	cancel = func() {} // Delay real context cancellation
	return e, nil
}

// WeaveletControl returns the controller component for the weavelet managed by this envelope.
func (e *EnvelopeGrpc) WeaveletControl() control.WeaveletControlGrpcClient { return e.controller }

// Serve accepts incoming messages from the weavelet.
func (e *EnvelopeGrpc) Serve(h control.DeployerControlGrpcServer) error {
	deployerAddr, err := net.Listen("tcp", e.weavelet.DeployerSocket)
	if err != nil {
		return err
	}

	var running errgroup.Group

	var stopErr error
	var once sync.Once
	stop := func(err error) {
		once.Do(func() {
			stopErr = err
		})
		e.ctxCancel()
	}

	// Capture stdout and stderr from the weavelet.
	if stdout := e.child.Stdout(); stdout != nil {
		running.Go(func() error {
			err := e.logLines("stdout", stdout, h)
			stop(err)
			return err
		})
	}
	if stderr := e.child.Stderr(); stderr != nil {
		running.Go(func() error {
			err := e.logLines("stderr", stderr, h)
			stop(err)
			return err
		})
	}

	// Start the goroutine watching the context for cancelation.
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
		control.RegisterDeployerControlGrpcServer(s, h)
		err := s.Serve(deployerAddr)
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
	stop(e.child.Wait())

	return stopErr
}

// Pid returns the process id of the weavelet, if it is running in a separate process.
func (e *EnvelopeGrpc) Pid() (int, bool) {
	return e.child.Pid()
}

// WeaveletAddress returns the address that other components should dial to communicate with the
// weavelet.
func (e *EnvelopeGrpc) WeaveletAddress() string {
	return e.weaveletAddr
}

// UpdateComponents updates the weavelet with the latest set of components it
// should be running.
func (e *EnvelopeGrpc) UpdateComponents(components []string) error {
	req := &protos.UpdateComponentsRequest{
		Components: components,
	}
	_, err := e.controller.UpdateComponents(context.TODO(), req)
	return err
}

// UpdateRoutingInfo updates the weavelet with a component's most recent
// routing info.
func (e *EnvelopeGrpc) UpdateRoutingInfo(routing *protos.RoutingInfo) error {
	req := &protos.UpdateRoutingInfoRequest{
		RoutingInfo: routing,
	}
	_, err := e.controller.UpdateRoutingInfo(context.TODO(), req)
	return err
}

func (e *EnvelopeGrpc) logLines(component string, src io.Reader, h control.DeployerControlGrpcServer) error {
	// Fill partial log entry.
	entry := &protos.LogEntry{
		App:       e.weavelet.App,
		Version:   e.weavelet.DeploymentId,
		Component: component,
		Node:      e.weavelet.Id,
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
