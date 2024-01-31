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

// An Event represents an atomic step of a execution.
type Event interface {
	isEvent()
}

// EventOpStart represents the start of an op.
type EventOpStart struct {
	TraceID int      // trace id
	SpanID  int      // span id
	Name    string   // op name
	Args    []string // op arguments
}

// EventOpFinish represents the finish of an op.
type EventOpFinish struct {
	TraceID int    // trace id
	SpanID  int    // span id
	Error   string // returned error message
}

// EventCall represents a component method call.
type EventCall struct {
	TraceID   int      // trace id
	SpanID    int      // span id
	Caller    string   // calling component (or "op")
	Replica   int      // calling component replica (or op number)
	Component string   // component being called
	Method    string   // method being called
	Args      []string // method arguments
}

// EventDeliverCall represents a component method call being delivered.
type EventDeliverCall struct {
	TraceID   int    // trace id
	SpanID    int    // span id
	Component string // component being called
	Replica   int    // component replica being called
}

// EventReturn represents a component method call returning.
type EventReturn struct {
	TraceID   int      // trace id
	SpanID    int      // span id
	Component string   // component returning
	Replica   int      // component replica returning
	Returns   []string // return values
}

// EventDeliverReturn represents the delivery of a method return.
type EventDeliverReturn struct {
	TraceID int // trace id
	SpanID  int // span id
}

// EventDeliverError represents the injection of an error.
type EventDeliverError struct {
	TraceID int // trace id
	SpanID  int // span id
}

// EventPanic represents a panic.
type EventPanic struct {
	TraceID  int    // trace id
	SpanID   int    // span id
	Panicker string // panicking component (or "op")
	Replica  int    // panicking component replica (or op number)
	Error    string // panic error
	Stack    string // stack trace
}

func (EventOpStart) isEvent()       {}
func (EventOpFinish) isEvent()      {}
func (EventCall) isEvent()          {}
func (EventDeliverCall) isEvent()   {}
func (EventReturn) isEvent()        {}
func (EventDeliverReturn) isEvent() {}
func (EventDeliverError) isEvent()  {}
func (EventPanic) isEvent()         {}

var _ Event = EventOpStart{}
var _ Event = EventOpFinish{}
var _ Event = EventCall{}
var _ Event = EventDeliverCall{}
var _ Event = EventReturn{}
var _ Event = EventDeliverReturn{}
var _ Event = EventDeliverError{}
var _ Event = EventPanic{}
