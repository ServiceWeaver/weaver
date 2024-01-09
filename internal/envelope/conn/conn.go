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

// Package conn implements a bi-directional communication channel between an
// envelope and a weavelet.
package conn

import (
	"io"
	sync "sync"

	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"google.golang.org/protobuf/proto"
)

// conn is a bi-directional communication channel that is used in the
// implementation of EnvelopeConn and WeaveletConn.
type conn struct {
	name   string
	reader io.ReadCloser

	mu      sync.Mutex
	writer  io.WriteCloser
	waiters map[int64]chan response // Response waiters
	failure error                   // Non-nil when error has been encountered
}

type response struct {
	result proto.Message
	err    error
}

func getId(msg proto.Message) int64 {
	switch x := msg.(type) {
	case *protos.WeaveletMsg:
		return x.Id
	case *protos.EnvelopeMsg:
		return x.Id
	default:
		return 0
	}
}

// recv reads the next request from the pipe and writes it to msg. Note that
// recv does NOT return RPC replies. These replies are returned directly the
// invoker of the RPC.
func (c *conn) recv(msg proto.Message) error {
	for {
		if err := protomsg.Read(c.reader, msg); err != nil {
			c.cleanup(err)
			return err
		}

		id := getId(msg)
		if id >= 0 {
			// This message is a request.
			return nil
		}
		// This message is an RPC reply.
		c.handleResponse(-id, protomsg.Clone(msg))
	}
}

func (c *conn) cleanup(err error) {
	// Wakeup all waiters.
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanupLocked(err)
}

func (c *conn) cleanupLocked(err error) {
	if c.failure != nil {
		return
	}
	c.failure = err
	for _, ch := range c.waiters {
		ch <- response{nil, err}
	}
	c.waiters = nil
	c.reader.Close()
	c.writer.Close()
}

func (c *conn) handleResponse(id int64, result proto.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch, ok := c.waiters[id]
	if !ok {
		return
	}
	delete(c.waiters, id)
	ch <- response{result, nil}
}

func (c *conn) send(msg proto.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return c.failure
	}

	var err error
	if err = protomsg.Write(c.writer, msg); err != nil {
		c.cleanupLocked(err)
	}
	return err
}
