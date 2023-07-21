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

package sim

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"reflect"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/config"
	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/internal/weaver"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
)

type Op[T any] struct {
	Name string
	Gen  func(*rand.Rand) T
	Func any
}

type Options struct {
	Seed           int64
	MaxReplicas    int
	NumOps         int
	ConfigFilename string
	Config         string
}

type Simulator struct {
	opts          Options
	registrations map[reflect.Type]*codegen.Registration
	components    map[string][]any
	ops           map[string]op

	ctx   context.Context
	group *errgroup.Group

	mu      sync.Mutex
	rand    *rand.Rand
	numOps  int
	calls   []*call
	replies []*reply
}

type op struct {
	t          reflect.Type
	name       string
	gen        reflect.Value
	f          reflect.Value
	components []reflect.Type
}

type call struct {
	component reflect.Type
	method    string
	args      []reflect.Value
	reply     chan *reply
}

type reply struct {
	call    *call
	returns []reflect.Value
}

func New(opts Options) (*Simulator, error) {
	// Index registrations.
	registrations := map[reflect.Type]*codegen.Registration{}
	for _, reg := range codegen.Registered() {
		registrations[reg.Iface] = reg
	}

	// Parse config.
	app := &protos.AppConfig{}
	if opts.Config != "" {
		var err error
		app, err = runtime.ParseConfig(opts.ConfigFilename, opts.Config, codegen.ComponentConfigValidator)
		if err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}

	// Create simulator.
	s := &Simulator{
		opts:          opts,
		registrations: registrations,
		components:    map[string][]any{},
		ops:           map[string]op{},
		rand:          rand.New(rand.NewSource(opts.Seed)),
	}

	// Create component replicas.
	for _, reg := range registrations {
		for i := 0; i < opts.MaxReplicas; i++ {
			// Create the component implementation.
			v := reflect.New(reg.Impl)
			obj := v.Interface()

			// Fill config.
			if cfg := config.Config(v); cfg != nil {
				if err := runtime.ParseConfigSection(reg.Name, "", app.Sections, cfg); err != nil {
					return nil, err
				}
			}

			// Set logger.
			//
			// TODO(mwhittaker): Use custom logger.
			if err := weaver.SetLogger(obj, slog.Default()); err != nil {
				return nil, err
			}

			// Fill ref fields.
			if err := weaver.FillRefs(obj, func(t reflect.Type) (any, error) {
				return s.getIntf(t)
			}); err != nil {
				return nil, err
			}

			// Fill listener fields.
			if err := weaver.FillListeners(obj, func(name string) (net.Listener, string, error) {
				lis, err := net.Listen("tcp", ":0")
				return lis, "", err
			}); err != nil {
				return nil, err
			}

			// Call Init if available.
			if i, ok := obj.(interface{ Init(context.Context) error }); ok {
				// TODO(mwhittaker): Use better context.
				if err := i.Init(context.Background()); err != nil {
					return nil, fmt.Errorf("component %q initialization failed: %w", reg.Name, err)
				}
			}

			s.components[reg.Name] = append(s.components[reg.Name], obj)
		}
	}

	return s, nil
}

func RegisterOp[T any](s *Simulator, o Op[T]) {
	// TODO(mwhittaker): Improve error messages.
	if _, ok := s.ops[o.Name]; ok {
		panic(fmt.Errorf("duplicate registration of op %q", o.Name))
	}
	t := reflect.TypeOf(o.Func)
	if t.Kind() != reflect.Func {
		panic(fmt.Errorf("op %q func is not a function: %T", o.Name, o.Func))
	}
	if t.NumIn() < 2 {
		panic(fmt.Errorf("op %q func has < 2 arguments: %T", o.Name, o.Func))
	}
	if t.In(0) != reflection.Type[context.Context]() {
		panic(fmt.Errorf("op %q func's first argument is not context.Context: %T", o.Name, o.Func))
	}
	if t.In(1) != reflection.Type[T]() {
		panic(fmt.Errorf("op %q func's second argument is not %v: %T", o.Name, reflection.Type[T](), o.Func))
	}
	var components []reflect.Type
	for i := 2; i < t.NumIn(); i++ {
		if _, ok := s.registrations[t.In(i)]; !ok {
			panic(fmt.Errorf("op %q func argument %d is not a registered component: %T", o.Name, i, o.Func))
		}
		components = append(components, t.In(i))
	}
	if t.NumOut() != 1 {
		panic(fmt.Errorf("op %q func does not have exactly one return: %T", o.Name, o.Func))
	}
	if t.Out(0) != reflection.Type[error]() {
		panic(fmt.Errorf("op %q func does not return an error: %T", o.Name, o.Func))
	}

	s.ops[o.Name] = op{
		t:          reflection.Type[T](),
		name:       o.Name,
		gen:        reflect.ValueOf(o.Gen),
		f:          reflect.ValueOf(o.Func),
		components: components,
	}
}

func (s *Simulator) getIntf(t reflect.Type) (any, error) {
	reg, ok := s.registrations[t]
	if !ok {
		return nil, fmt.Errorf("component %v not found", t)
	}
	return reg.ReflectStubFn(s.call), nil
}

func (s *Simulator) Simulate(ctx context.Context) error {
	s.group, s.ctx = errgroup.WithContext(ctx)
	s.step()
	return s.group.Wait()
}

func (s *Simulator) step() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Compute the set of candidate steps.
	const runOp = 0
	const deliverCall = 1
	const deliverReply = 2
	var candidates []int
	if s.numOps < s.opts.NumOps {
		candidates = append(candidates, runOp)
	}
	if len(s.calls) > 0 {
		candidates = append(candidates, deliverCall)
	}
	if len(s.replies) > 0 {
		candidates = append(candidates, deliverReply)
	}
	if len(candidates) == 0 {
		return
	}

	// Randomly execute a step.
	switch x := candidates[s.rand.Intn(len(candidates))]; x {
	case runOp:
		s.numOps++
		o := pickValue(s.rand, s.ops)
		s.group.Go(func() error {
			return s.runOp(s.ctx, o)
		})

	case deliverCall:
		var call *call
		call, s.calls = pop(s.rand, s.calls)
		s.group.Go(func() error {
			s.deliverCall(call)
			return nil
		})

	case deliverReply:
		var reply *reply
		reply, s.replies = pop(s.rand, s.replies)
		reply.call.reply <- reply

	default:
		panic(fmt.Errorf("unrecognized candidate %v", x))
	}
}

func (s *Simulator) runOp(ctx context.Context, o op) error {
	s.mu.Lock()
	val := o.gen.Call([]reflect.Value{reflect.ValueOf(s.rand)})[0]
	s.mu.Unlock()

	args := make([]reflect.Value, 2+len(o.components))
	args[0] = reflect.ValueOf(ctx)
	args[1] = val
	for i, component := range o.components {
		c, err := s.getIntf(component)
		if err != nil {
			return err
		}
		args[2+i] = reflect.ValueOf(c)
	}

	if x := o.f.Call(args)[0].Interface(); x != nil {
		return x.(error)
	}
	s.step()
	return nil
}

func (s *Simulator) call(component reflect.Type, method string, args []reflect.Value) []reflect.Value {
	reply := make(chan *reply, 1)
	s.mu.Lock()
	s.calls = append(s.calls, &call{
		component: component,
		method:    method,
		args:      args,
		reply:     reply,
	})
	s.mu.Unlock()
	s.step()
	return (<-reply).returns
}

func (s *Simulator) deliverCall(call *call) {
	reg, ok := s.registrations[call.component]
	if !ok {
		panic(fmt.Errorf("component %v not found", call.component))
	}
	s.mu.Lock()
	replica := pick(s.rand, s.components[reg.Name])
	s.mu.Unlock()

	returns := reflect.ValueOf(replica).MethodByName(call.method).Call(call.args)
	s.mu.Lock()
	s.replies = append(s.replies, &reply{
		call:    call,
		returns: returns,
	})
	s.mu.Unlock()

	s.step()
}
