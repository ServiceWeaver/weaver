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

// Package cond implements a context-aware condition variable.
package cond

import (
	"context"
	"sync"
)

// # Implementation Overview
//
// When a goroutine calls cond.Wait(ctx), Wait creates a channel and appends it
// to a queue of waiting channels inside of cond. It then performs a select on
// ctx.Done and the newly minted channel. Signal pops the first waiting channel
// and closes it. Broadcast pops and closes every waiting channel.

// Cond is a context-aware version of a sync.Cond. Like a sync.Cond, a Cond
// must not be copied after first use.
type Cond struct {
	L sync.Locker

	// Note that we need our own mutex instead of using L because Signal and
	// Broadcast can be called without holding L.
	m       sync.Mutex
	waiters []chan struct{}
}

// NewCond returns a new Cond with Locker l.
func NewCond(l sync.Locker) *Cond {
	return &Cond{L: l}
}

// Broadcast is identical to sync.Cond.Broadcast.
func (c *Cond) Broadcast() {
	c.m.Lock()
	defer c.m.Unlock()
	for _, wait := range c.waiters {
		close(wait)
	}
	c.waiters = nil
}

// Signal is identical to sync.Cond.Signal.
func (c *Cond) Signal() {
	c.m.Lock()
	defer c.m.Unlock()
	if len(c.waiters) == 0 {
		return
	}
	wait := c.waiters[0]
	c.waiters = c.waiters[1:]
	close(wait)
}

// Wait behaves identically to sync.Cond.Wait, except that it respects the
// provided context. Specifically, if the context is cancelled, c.L is
// reacquired and ctx.Err() is returned. Example usage:
//
//	for !condition() {
//	    if err := cond.Wait(ctx); err != nil {
//	        // The context was cancelled. cond.L is locked at this point.
//	        return err
//	    }
//	    // Wait returned normally. cond.L is still locked at this point.
//	}
func (c *Cond) Wait(ctx context.Context) error {
	wait := make(chan struct{})
	c.m.Lock()
	c.waiters = append(c.waiters, wait)
	c.m.Unlock()

	c.L.Unlock()
	var err error
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case <-wait:
	}
	c.L.Lock()
	return err
}
