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

package call

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
)

// An endpoint is a dialable entity with an address. For example,
// TCP("localhost:8000") returns an endpoint that dials the TCP server running
// on localhost:8000, and Unix("/tmp/unix.sock") returns an endpoint that dials
// the Unix socket /tmp/unix.sock.
type Endpoint interface {
	// Dial returns an network connection to the endpoint.
	Dial(ctx context.Context) (net.Conn, error)

	// Address returns the address of the endpoint. If two endpoints have the
	// same address, then they are guaranteed to represent the same endpoint.
	// But, two endpoints with different addresses may also represent the same
	// endpoint (e.g., TCP("golang.org:http") and TCP("golang.org:80")).
	Address() string
}

// TCP returns a TCP endpoint. The provided address is passed to net.Dial. For
// example:
//
//	TCP("golang.org:http")
//	TCP("192.0.2.1:http")
//	TCP("198.51.100.1:80")
//	TCP("[2001:db8::1]:domain")
//	TCP("[fe80::1%lo0]:53")
//	TCP(":80")
func TCP(address string) NetEndpoint {
	return NetEndpoint{"tcp", address}
}

// Unix returns an endpoint that uses Unix sockets. The provided filename is
// passed to net.Dial. For example:
//
//	Unix("unix.sock")
//	Unix("/tmp/unix.sock")
func Unix(filename string) NetEndpoint {
	return NetEndpoint{"unix", filename}
}

// MTLS returns an endpoint that performs MTLS authentication over the
// underlying endpoint. For example:
//
//	MTLS(&tls.Config{...}, TCP("golang.org:http"))
//	MTLS(&tls.Config{...}, Unix("unix.sock"))
//
// REQUIRES: config is not nil
func MTLS(config *tls.Config, ep Endpoint) Endpoint {
	return &tlsEndpoint{config: config, ep: ep}
}

// NetEndpoint is an Endpoint that implements Dial using net.Dial.
type NetEndpoint struct {
	Net  string // e.g., "tcp", "udp", "unix"
	Addr string // e.g., "localhost:8000", "/tmp/unix.sock"
}

// Check that NetEndpoint implements the Endpoint interface.
var _ Endpoint = NetEndpoint{}

// Dial implements the Endpoint interface.
func (ne NetEndpoint) Dial(ctx context.Context) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, ne.Net, ne.Addr)
}

// Address implements the Endpoint interface.
func (ne NetEndpoint) Address() string {
	return fmt.Sprintf("%s://%s", ne.Net, ne.Addr)
}

func (ne NetEndpoint) String() string {
	return ne.Address()
}

// ParseNetEndpoint parses a string with a format of net://addr into a
// NetAddress. For example,
//
//	ParseNetEndpoint("tcp://localhost:80") // NetEndpoint{"tcp", "localhost:80"}
//	ParseNetEndpoint("unix://unix.sock")   // NetEndpoint{"unix", "unix.sock"}
func ParseNetEndpoint(endpoint string) (NetEndpoint, error) {
	net, addr, ok := strings.Cut(endpoint, "://")
	if !ok {
		return NetEndpoint{}, fmt.Errorf("%q does not have format <network>://<address>", endpoint)
	}
	return NetEndpoint{Net: net, Addr: addr}, nil
}

type tlsEndpoint struct {
	config *tls.Config
	ep     Endpoint
}

var _ Endpoint = &tlsEndpoint{}

// Dial implements the Endpoint interface.
func (t *tlsEndpoint) Dial(ctx context.Context) (net.Conn, error) {
	conn, err := t.ep.Dial(ctx)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.Client(conn, t.config)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}
	return tlsConn, nil
}

// Address implements the Endpoint interface.
func (t *tlsEndpoint) Address() string {
	return fmt.Sprintf("mtls://%s", t.ep.Address())
}

func (t *tlsEndpoint) String() string {
	return t.Address()
}
