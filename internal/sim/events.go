package sim

import (
	"fmt"
)

// Instance names a particular op or component replica.
type Instance struct {
	Name  string // component name or "Op"
	Index int    // component replica index or op id
}

func (i Instance) String() string {
	return fmt.Sprintf("%s %d", i.Name, i.Index)
}

// An Event is a single event in a simulation. Events include
//
// - an op starting,
// - an op finishing,
// - an op or component sending a request (aka calling a method),
// - a component receiving a request (aka receiving a method call),
// - a component sending a reply (aka returning from a method call), and
// - an op or component receiving a reply (aka receiving a method return).
type Event interface {
	isEvent()
}

type OpStartEvent struct {
	OpId   int
	SpanId int
	Name   string
	Args   []any
}

type OpFinishEvent struct {
	OpId   int
	SpanId int
	Err    error
}

type SendRequestEvent struct {
	OpId      int
	SpanId    int
	Requester Instance
	Component string
	Method    string
	Args      []any
}

type ReceiveRequestEvent struct {
	OpId      int
	SpanId    int
	Requester Instance
	Replier   Instance
	Component string
	Method    string
	Args      []any
}

type SendReplyEvent struct {
	OpId      int
	SpanId    int
	Requester Instance
	Replier   Instance
	Reply     any
	Err       error
}

type ReceiveReplyEvent struct {
	OpId      int
	SpanId    int
	Requester Instance
	Replier   Instance
	Reply     any
	Err       error
}

func (OpStartEvent) isEvent()        {}
func (OpFinishEvent) isEvent()       {}
func (SendRequestEvent) isEvent()    {}
func (ReceiveRequestEvent) isEvent() {}
func (SendReplyEvent) isEvent()      {}
func (ReceiveReplyEvent) isEvent()   {}
