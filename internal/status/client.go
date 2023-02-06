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

package status

import (
	"context"
	"net/http"

	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	protos "github.com/ServiceWeaver/weaver/runtime/protos"
)

// Client is an HTTP client to a status server. It's assumed the status server
// registered itself with RegisterServer.
type Client struct {
	addr string // status server (e.g., "localhost:12345")
}

var _ Server = &Client{}

// NewClient returns a client to the status server on the provided address.
func NewClient(addr string) *Client {
	return &Client{addr}
}

// Status implements the Server interface.
func (c *Client) Status(ctx context.Context) (*Status, error) {
	status := &Status{}
	err := protomsg.Call(ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    "http://" + c.addr,
		URLPath: statusEndpoint,
		Reply:   status,
	})
	if err != nil {
		return nil, err
	}
	status.StatusAddr = c.addr
	return status, nil
}

// Metrics implements the Server interface.
func (c *Client) Metrics(ctx context.Context) (*Metrics, error) {
	metrics := &Metrics{}
	err := protomsg.Call(ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    "http://" + c.addr,
		URLPath: metricsEndpoint,
		Reply:   metrics,
	})
	return metrics, err
}

// Profile implements the Server interface.
func (c *Client) Profile(ctx context.Context, req *protos.RunProfiling) (*protos.Profile, error) {
	profile := &protos.Profile{}
	err := protomsg.Call(ctx, protomsg.CallArgs{
		Client:  http.DefaultClient,
		Addr:    "http://" + c.addr,
		URLPath: profileEndpoint,
		Request: req,
		Reply:   profile,
	})
	return profile, err
}
