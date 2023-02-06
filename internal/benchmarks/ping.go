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

package benchmarks

import (
	"context"

	weaver "github.com/ServiceWeaver/weaver"
)

//go:generate ../../cmd/weaver/weaver generate
//go:generate ../../dev/protoc.sh payload.proto

// We want a chain of ten components, each one calling the next.

// pingCommon holds the shared component implementation.
type pingCommon struct {
	next Ping
}

// setNext initializes p.next to point to the component of type Next.
func setNext[Next Ping](p *pingCommon, requestor weaver.Instance) error {
	next, err := weaver.Get[Next](requestor)
	p.next = next
	return err
}

func (p *pingCommon) PingC(ctx context.Context, req payloadC, depth int) (payloadC, error) {
	if depth == 1 {
		return req, nil
	}
	return p.next.PingC(ctx, req, depth-1)
}

func (p *pingCommon) PingS(ctx context.Context, req payloadS, depth int) (payloadS, error) {
	if depth == 1 {
		return req, nil
	}
	return p.next.PingS(ctx, req, depth-1)
}

// Ping is the shared interface implemented by all components.
type Ping interface {
	PingC(ctx context.Context, req payloadC, depth int) (payloadC, error)
	PingS(ctx context.Context, req payloadS, depth int) (payloadS, error)
}

type Ping1 interface{ Ping }
type Ping2 interface{ Ping }
type Ping3 interface{ Ping }
type Ping4 interface{ Ping }
type Ping5 interface{ Ping }
type Ping6 interface{ Ping }
type Ping7 interface{ Ping }
type Ping8 interface{ Ping }
type Ping9 interface{ Ping }
type Ping10 interface{ Ping }

type ping1 struct {
	weaver.Implements[Ping1]
	pingCommon
}

type ping2 struct {
	weaver.Implements[Ping2]
	pingCommon
}

type ping3 struct {
	weaver.Implements[Ping3]
	pingCommon
}

type ping4 struct {
	weaver.Implements[Ping4]
	pingCommon
}

type ping5 struct {
	weaver.Implements[Ping5]
	pingCommon
}

type ping6 struct {
	weaver.Implements[Ping6]
	pingCommon
}

type ping7 struct {
	weaver.Implements[Ping7]
	pingCommon
}

type ping8 struct {
	weaver.Implements[Ping8]
	pingCommon
}

type ping9 struct {
	weaver.Implements[Ping9]
	pingCommon
}

type ping10 struct {
	weaver.Implements[Ping10]
	pingCommon
}

func (p *ping1) Init(context.Context) error { return setNext[Ping2](&p.pingCommon, p) }
func (p *ping2) Init(context.Context) error { return setNext[Ping3](&p.pingCommon, p) }
func (p *ping3) Init(context.Context) error { return setNext[Ping4](&p.pingCommon, p) }
func (p *ping4) Init(context.Context) error { return setNext[Ping5](&p.pingCommon, p) }
func (p *ping5) Init(context.Context) error { return setNext[Ping6](&p.pingCommon, p) }
func (p *ping6) Init(context.Context) error { return setNext[Ping7](&p.pingCommon, p) }
func (p *ping7) Init(context.Context) error { return setNext[Ping8](&p.pingCommon, p) }
func (p *ping8) Init(context.Context) error { return setNext[Ping9](&p.pingCommon, p) }
func (p *ping9) Init(context.Context) error { return setNext[Ping10](&p.pingCommon, p) }

// ping10 has no next component.

// payloadS contains a simple payload.
type payloadS struct {
	weaver.AutoMarshal
	Values []string
}

// payloadC defines a payload that emulates some production payload.
type payloadC struct {
	weaver.AutoMarshal
	A float64
	B string
	C int64
	D X1
	E string
	F int64
	G X6
	H string
	I int64
	J float32
	K string
}

type X1 struct {
	weaver.AutoMarshal
	A X2
	B []int64
}

type X2 struct {
	weaver.AutoMarshal
	A X3
}

type X3 struct {
	weaver.AutoMarshal
	A X4
	B int64
	C int64
}

type X4 struct {
	weaver.AutoMarshal
	A int64
	B X5
	C int64
}

type X5 struct {
	weaver.AutoMarshal
	A int64
	B int64
}

type X6 struct {
	weaver.AutoMarshal
	A []bool
}

// Functions to convert to proto.

func (p *payloadC) ToProto() *PayloadCProto {
	return &PayloadCProto{
		A: p.A, B: p.B, C: p.C, D: p.D.ToProto(), E: p.E, F: p.F,
		G: p.G.ToProto(), H: p.H, I: p.I, J: p.J, K: p.K,
	}
}
func (x *X1) ToProto() *X1Proto { return &X1Proto{A: x.A.ToProto(), B: x.B} }
func (x *X2) ToProto() *X2Proto { return &X2Proto{A: x.A.ToProto()} }
func (x *X3) ToProto() *X3Proto { return &X3Proto{A: x.A.ToProto(), B: int64(x.B), C: int64(x.C)} }
func (x *X4) ToProto() *X4Proto { return &X4Proto{A: int64(x.A), B: x.B.ToProto(), C: int64(x.C)} }
func (x *X5) ToProto() *X5Proto { return &X5Proto{A: int64(x.A), B: int64(x.B)} }
func (x *X6) ToProto() *X6Proto { return &X6Proto{A: x.A} }

// Functions to convert from proto.

func (p *payloadC) FromProto(proto *PayloadCProto) {
	p.A = proto.A
	p.B = proto.B
	p.C = proto.C
	p.D.FromProto(proto.D)
	p.E = proto.E
	p.F = proto.F
	p.G.FromProto(proto.G)
	p.H = proto.H
	p.I = proto.I
	p.J = proto.J
	p.K = proto.K
}

func (x *X1) FromProto(p *X1Proto) {
	x.A.FromProto(p.A)
	x.B = p.B
}

func (x *X2) FromProto(p *X2Proto) {
	x.A.FromProto(p.A)
}

func (x *X3) FromProto(p *X3Proto) {
	x.A.FromProto(p.A)
	x.B = p.B
	x.C = p.C
}

func (x *X4) FromProto(p *X4Proto) {
	x.A = p.A
	x.B.FromProto(p.B)
	x.C = p.C
}

func (x *X5) FromProto(p *X5Proto) {
	x.A = p.A
	x.B = p.B
}

func (x *X6) FromProto(p *X6Proto) {
	x.A = p.A
}
