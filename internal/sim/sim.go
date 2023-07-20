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
	"github.com/ServiceWeaver/weaver/internal/weaver"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"golang.org/x/exp/slog"
)

type Options struct {
	Seed           int64
	MaxReplicas    int
	ConfigFilename string
	Config         string
}

type Simulator struct {
	opts       Options
	config     *protos.AppConfig
	regs       []*codegen.Registration
	regsByIntf map[reflect.Type]*codegen.Registration
	components map[string][]any

	mu   sync.Mutex
	rand *rand.Rand
}

func New(opts Options) (*Simulator, error) {
	// Index registrations.
	regs := codegen.Registered()
	regsByIntf := map[reflect.Type]*codegen.Registration{}
	for _, reg := range regs {
		regsByIntf[reg.Iface] = reg
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
		opts:       opts,
		config:     app,
		regs:       regs,
		regsByIntf: regsByIntf,
		components: map[string][]any{},
		rand:       rand.New(rand.NewSource(opts.Seed)),
	}

	// Create component replicas.
	for _, reg := range regs {
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
				return s.GetIntf(t)
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

func (s *Simulator) GetIntf(t reflect.Type) (any, error) {
	reg, ok := s.regsByIntf[t]
	if !ok {
		return nil, fmt.Errorf("component %v not found", t)
	}
	return reg.ReflectStubFn(s.call), nil
}

func (s *Simulator) call(component reflect.Type, method string, args []reflect.Value) []reflect.Value {
	reg, ok := s.regsByIntf[component]
	if !ok {
		panic(fmt.Errorf("component %v not found", component))
	}

	replicas := s.components[reg.Name]
	s.mu.Lock()
	i := s.rand.Intn(len(replicas))
	s.mu.Unlock()
	return reflect.ValueOf(replicas[i]).MethodByName(method).Call(args)
}
