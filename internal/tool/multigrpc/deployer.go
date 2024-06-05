package multigrpc

import (
	"context"
	"net"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/control"
	"github.com/ServiceWeaver/weaver/internal/tool/multi"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/envelope"
	"github.com/ServiceWeaver/weaver/runtime/grpcregistry"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"golang.org/x/exp/maps"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"
)

const defaultReplication = 2

type deployer struct {
	control.DeployerControlGrpcServer

	ctx       context.Context
	ctxCancel context.CancelFunc
	config    *multi.MultiConfig
	running   errgroup.Group

	mu     sync.Mutex        // guards the following
	err    error             // error that stopped the babysitter
	groups map[string]*group // groups, by group name
}

// A group contains information about a component being deployed.
type group struct {
	name      string                   // group name
	started   bool                     // started components
	envelopes []*envelope.EnvelopeGrpc // envelopes, one per weavelet
	replicas  map[string]bool          // weavelet addresses
}

// handler handles a connection to a weavelet.
type handler struct {
	*deployer
	g        *group
	envelope *envelope.EnvelopeGrpc
}

func newDeployer(ctx context.Context, config *multi.MultiConfig) (*deployer, error) {
	ctx, cancel := context.WithCancel(ctx)
	d := &deployer{
		ctx:       ctx,
		ctxCancel: cancel,
		config:    config,
		groups:    map[string]*group{},
	}

	groups := grpcregistry.Registered()
	for _, g := range groups {
		d.groups[g.Name] = &group{
			name:     g.Name,
			replicas: map[string]bool{},
		}
	}
	d.groups[runtime.Main] = &group{name: runtime.Main, replicas: map[string]bool{}}

	// Start a goroutine that watches for context cancellation.
	d.running.Go(func() error {
		<-d.ctx.Done()
		err := d.ctx.Err()
		d.stop(err)
		return err
	})

	return d, nil
}

func (d *deployer) startMain() error {
	return d.activateComponent(&protos.ActivateComponentRequest{Component: runtime.Main})
}

func (d *deployer) activateComponent(req *protos.ActivateComponentRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.groups[req.Component]; !ok {
		d.groups[req.Component] = &group{name: req.Component, replicas: map[string]bool{}}
	}
	g := d.groups[req.Component]
	if g.started {
		return nil
	}
	return d.startGroup(g)
}

func (d *deployer) startGroup(g *group) error {
	if d.err != nil {
		return d.err
	}

	rep := defaultReplication
	if g.name == runtime.Main {
		rep = 1
	}

	if len(g.envelopes) == rep {
		// Already started.
		return nil
	}

	for r := 0; r < rep; r++ {
		ctrlSocket, err := getControlSocket()
		if err != nil {
			return err
		}

		// Start the weavelet.
		args := &protos.WeaveletArgs{
			App:             d.config.App.Name,
			RunMain:         g.name == runtime.Main,
			InternalAddress: "127.0.0.1:0",
			ControlSocket:   ctrlSocket,
		}

		e, err := envelope.NewEnvelopeGrpc(d.ctx, args, d.config.App)
		if err != nil {
			return err
		}

		h := &handler{
			deployer: d,
			g:        g,
			envelope: e,
		}
		d.running.Go(func() error {
			err := e.Serve(h)
			d.stop(err)
			return err
		})
		pid, ok := e.Pid()
		if !ok {
			panic("multi deployer child must be a real process")
		}

		if err := d.registerReplica(g, e.WeaveletAddress(), pid); err != nil {
			return err
		}
		if err := e.UpdateComponents([]string{g.name}); err != nil {
			return err
		}

		// Update routing info about other things.
		// Notify all groups about changes in this group.
		for gn, gr := range d.groups {
			if gn == g.name || gn == runtime.Main {
				continue
			}
			if err := e.UpdateRoutingInfo(
				&protos.RoutingInfo{
					Component: gn,
					Replicas:  maps.Keys(gr.replicas),
				}); err != nil {
				return err
			}
		}

		g.envelopes = append(g.envelopes, e)
	}
	g.started = true // Robert - is this the place to mark it as started???
	return nil
}

// wait waits for the deployer to terminate
func (d *deployer) wait() error {
	d.running.Wait()
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.err
}

func (d *deployer) stop(err error) {
	// Record the first error.
	d.mu.Lock()
	if d.err == nil {
		d.err = err
	}
	d.mu.Unlock()

	// Cancel the context.
	d.ctxCancel()
}

func (d *deployer) registerReplica(g *group, replicaAddr string, pid int) error {
	if g.replicas[replicaAddr] {
		return nil // Replica already registered
	}
	g.replicas[replicaAddr] = true

	// Notify all groups about changes in this group.
	for _, gr := range d.groups {
		for _, e := range gr.envelopes {
			if err := e.UpdateRoutingInfo(
				&protos.RoutingInfo{
					Component: g.name,
					Replicas:  maps.Keys(g.replicas),
				}); err != nil {
				return err
			}
		}
	}
	return nil
}

func getControlSocket() (string, error) {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", err
	}
	defer lis.Close()
	return lis.Addr().String(), nil
}

func (h *handler) ActivateComponent(ctx context.Context, request *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error) {
	return &protos.ActivateComponentReply{}, h.activateComponent(request)
}

func (h *handler) GetListenerAddress(ctx context.Context, request *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
	//TODO implement me
	panic("implement me")
}

func (h *handler) ExportListener(ctx context.Context, request *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	//TODO implement me
	panic("implement me")
}

func (h *handler) GetSelfCertificate(ctx context.Context, request *protos.GetSelfCertificateRequest) (*protos.GetSelfCertificateReply, error) {
	//TODO implement me
	panic("implement me")
}

func (h *handler) VerifyClientCertificate(ctx context.Context, request *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error) {
	//TODO implement me
	panic("implement me")
}

func (h *handler) VerifyServerCertificate(ctx context.Context, request *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error) {
	//TODO implement me
	panic("implement me")
}

func (h *handler) LogBatch(ctx context.Context, batch *protos.LogEntryBatch) (*emptypb.Empty, error) {
	//TODO implement me
	panic("implement me")
}

func (h *handler) HandleTraceSpans(ctx context.Context, spans *protos.TraceSpans) (*emptypb.Empty, error) {
	//TODO implement me
	panic("implement me")
}
