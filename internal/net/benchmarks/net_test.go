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

package net

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/logging"
)

var (
	subproccess           = flag.Bool("subprocess", false, "Is this a subprocess?")
	network               = flag.String("network", "", "Network (e.g., tcp, unix, mtls)")
	address               = flag.String("address", "", "Server address")
	inlineHandlerDuration = flag.Duration("inline_handler_duration", -1, "ServerOptions.InlineHandlerDuration")
	writeFlattenLimit     = flag.Int("write_flatten_limit", -1, "ServerOptions.WriteFlattenLimit")

	echoKey   = call.MakeMethodKey("component", "echo")
	handlers  = call.NewHandlerMap()
	tlsConfig = makeTLSConfig()
)

func makeTLSConfig() *tls.Config {
	certPem := []byte(`-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----`)
	keyPem := []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIrYSSNQFaA2Hwf1duRSxKtLYX5CB04fSeQ6tF1aY/PuoAoGCCqGSM49
AwEHoUQDQgAEPR3tU2Fta9ktY+6P9G0cWO+0kETA6SFs38GecTyudlHz6xvCdz8q
EKTcWGekdmdDPsHloRNtsiCa697B2O9IFA==
-----END EC PRIVATE KEY-----`)
	cert, err := tls.X509KeyPair(certPem, keyPem)
	if err != nil {
		log.Fatal(err)
	}
	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		ClientAuth:         tls.RequireAnyClientCert,
		InsecureSkipVerify: true, // ok when VerifyPeerCertificate present
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) != 1 {
				return fmt.Errorf("expected single cert, got %d", len(rawCerts))
			}
			if !bytes.Equal(cert.Certificate[0], rawCerts[0]) {
				return fmt.Errorf("invalid peer certificate")
			}
			return nil
		},
	}
}

type testListener struct {
	net.Listener
	tlsConfig *tls.Config
}

var _ call.Listener = &testListener{}

func (l testListener) Accept() (net.Conn, *call.HandlerMap, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, nil, err
	}
	if l.tlsConfig != nil {
		tlsConn := tls.Server(conn, l.tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return nil, nil, fmt.Errorf("TLS handshake error: %w", err)
		}
		conn = tlsConn
	}
	return conn, handlers, err
}

func TestMain(m *testing.M) {
	handlers.Set("component", "echo", func(_ context.Context, args []byte) ([]byte, error) {
		return args, nil
	})

	flag.Parse()
	if !*subproccess {
		os.Exit(m.Run())
	}

	// Some tests launch a server as a subprocess.
	config := config{
		network:               *network,
		addr:                  *address,
		inlineHandlerDuration: *inlineHandlerDuration,
		writeFlattenLimit:     *writeFlattenLimit,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := config.serverOpts()
	opts.Logger = logging.StderrLogger(logging.Options{})
	if err := call.Serve(ctx, config.listen(), opts); err != nil {
		panic(err)
	}
}

// config configures a benchmark.
type config struct {
	network                string        // e.g., tcp, unix, mtls
	addr                   string        // e.g., localhost:12345, /tmp/socket
	optimisticSpinDuration time.Duration // call.ClientOptions
	inlineHandlerDuration  time.Duration // call.ServerOptions
	writeFlattenLimit      int           // call.ClientOptions and call.ServerOptions
}

// name returns a benchmark name.
func (c config) name() string {
	return fmt.Sprintf(
		"%s/Spin=%v/Inline=%v/Flatten=%v",
		c.network,
		c.optimisticSpinDuration,
		c.inlineHandlerDuration,
		c.writeFlattenLimit,
	)
}

// listen returns a net.Listener suitable for a server.
func (c config) listen() call.Listener {
	switch c.network {
	case "tcp":
		l, err := net.Listen("tcp", c.addr)
		if err != nil {
			panic(err)
		}
		return testListener{Listener: l}
	case "unix":
		l, err := net.Listen("unix", c.addr)
		if err != nil {
			panic(err)
		}
		return testListener{Listener: l}
	case "mtls":
		l, err := net.Listen("tcp", c.addr)
		if err != nil {
			panic(err)
		}
		return testListener{Listener: l, tlsConfig: tlsConfig}
	default:
		panic(fmt.Sprintf("invalid network: %s", c.network))
	}
}

// endpoint returns a call.Endpoint suitable for a client.
func (c config) endpoint() call.Endpoint {
	switch c.network {
	case "tcp":
		return call.TCP(c.addr)
	case "unix":
		return call.Unix(c.addr)
	case "mtls":
		return call.MTLS(tlsConfig, call.TCP(c.addr))
	default:
		panic(fmt.Sprintf("invalid network: %s", c.network))
	}
}

// clientOpts returns the client options.
func (c config) clientOpts() call.ClientOptions {
	return call.ClientOptions{
		OptimisticSpinDuration: c.optimisticSpinDuration,
		WriteFlattenLimit:      c.writeFlattenLimit,
	}
}

// serverOpts returns the server options.
func (c config) serverOpts() call.ServerOptions {
	return call.ServerOptions{
		InlineHandlerDuration: c.inlineHandlerDuration,
		WriteFlattenLimit:     c.writeFlattenLimit,
	}
}

// configs returns a set of configs.
func configs(b testing.TB) []config {
	var configs []config
	for _, network := range []string{"tcp", "unix", "mtls"} {
		for _, spin := range []time.Duration{0, 20 * time.Microsecond} {
			for _, inline := range []time.Duration{-1, 20 * time.Microsecond} {
				for _, flatten := range []int{-1, 1 << 10, 4 << 10} {
					addr := "localhost:12345"
					if network == "unix" {
						addr = filepath.Join(b.TempDir(), "socket")
					}
					configs = append(configs, config{
						network:                network,
						addr:                   addr,
						optimisticSpinDuration: spin,
						inlineHandlerDuration:  inline,
						writeFlattenLimit:      flatten,
					})
				}
			}
		}
	}
	return configs
}

// serve starts an echo RPC server (in a separate goroutine) that listens on
// the provided test.network (e.g., tcp, unix) and test.address (e.g.
// localhost:12345, /tmp/socket). The server is shut down when the provided
// benchmark ends.
func serve(b *testing.B, config config) {
	// Kill the server to free up the port it's listening on.
	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)

	// Launch the server.
	go func() {
		opts := config.serverOpts()
		opts.Logger = logging.NewTestSlogger(b, testing.Verbose())
		switch err := call.Serve(ctx, config.listen(), opts); err {
		case nil, ctx.Err():
		case err:
			b.Log(err)
		}
	}()

	// Give the server time to start.
	time.Sleep(1 * time.Second)
}

// serveSubprocess starts an echo RPC server (in a separate process) that
// listens on the provided test.network (e.g., tcp, unix) and test.address
// (e.g. localhost:12345, /tmp/socket). The process is killed when the provided
// benchmark ends.
func serveSubprocess(b *testing.B, config config) {
	ex, err := os.Executable()
	if err != nil {
		b.Fatal(err)
	}
	cmd := exec.Command(ex,
		"--subprocess",
		"--network", config.network,
		"--address", config.addr,
		fmt.Sprintf("--inline_handler_duration=%v", config.inlineHandlerDuration),
		fmt.Sprintf("--write_flatten_limit=%v", config.writeFlattenLimit),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := cmd.Process.Kill(); err != nil {
			b.Log(err)
		}
	})
	time.Sleep(1 * time.Second) // Give the server time to start.
}

// echo executes an echo RPC.
func echo(ctx context.Context, client call.Connection, args []byte) (result []byte, err error) {
	defer func() {
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
	}()
	enc := codegen.NewEncoder()
	enc.Bytes(args)
	out, err := client.Call(ctx, echoKey, enc.Data(), call.CallOptions{})
	if err != nil {
		return out, err
	}
	return codegen.NewDecoder(out).Bytes(), nil
}

// benchmarkEchoClient benchmarks an echoClient with various message sizes.
func benchmarkEchoClient(b *testing.B, client call.Connection) {
	b.Helper()
	ctx := context.Background()
	sizes := []int{
		1,           // 1 byte
		100,         // 100 bytes
		1024,        // 1 Kib
		10 * 1024,   // 10 Kib
		100 * 1024,  // 100 Kib
		1024 * 1024, // 1 Mib
	}
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Msg-%s", sizeString(size)), func(b *testing.B) {
			want := []byte(strings.Repeat("x", size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got, err := echo(ctx, client, want)
				if err != nil {
					b.Fatal(err)
				}
				if len(got) != len(want) {
					b.Fatalf("wrong length: got %d, want %d", len(got), len(want))
				}
			}
		})
	}
}

// BenchmarkPipeRPC benchmarks an echo RPC over a net.Pipe.
func BenchmarkPipeRPC(b *testing.B) {
	// Start the server.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()
	sopts := call.ServerOptions{Logger: logging.NewTestSlogger(b, testing.Verbose())}
	call.ServeOn(ctx, s, handlers, sopts)

	// Create the client.
	resolver := call.NewConstantResolver(&connEndpoint{"client", c})
	copts := call.ClientOptions{Logger: logging.NewTestSlogger(b, testing.Verbose())}
	client, err := call.Connect(ctx, resolver, copts)
	if err != nil {
		b.Fatal(err)
	}
	benchmarkEchoClient(b, client)
}

// BenchmarkLocalRPC benchmarks echo calls between a client and server in the
// same process.
func BenchmarkLocalRPC(b *testing.B) {
	for _, config := range configs(b) {
		b.Run(config.name(), func(b *testing.B) {
			serve(b, config)
			resolver := call.NewConstantResolver(config.endpoint())
			opts := config.clientOpts()
			opts.Logger = logging.NewTestSlogger(b, testing.Verbose())
			client, err := call.Connect(context.Background(), resolver, opts)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkEchoClient(b, client)
		})
	}
}

// BenchmarkMultiprocRPC benchmarks echo calls between a client and server in
// different processes.
func BenchmarkMultiprocRPC(b *testing.B) {
	for _, config := range configs(b) {
		b.Run(config.name(), func(b *testing.B) {
			serveSubprocess(b, config)
			resolver := call.NewConstantResolver(config.endpoint())
			opts := config.clientOpts()
			opts.Logger = logging.NewTestSlogger(b, testing.Verbose())
			client, err := call.Connect(context.Background(), resolver, opts)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkEchoClient(b, client)
		})
	}
}

// connEndpoint is an endpoint that always returns the provided net.Conn.
type connEndpoint struct {
	name string
	conn net.Conn
}

// Dial implements the call.Endpoint interface.
func (c *connEndpoint) Dial(context.Context) (net.Conn, error) {
	return c.conn, nil
}

// Address implements the call.Endpoint interface.
func (c *connEndpoint) Address() string {
	return fmt.Sprintf("conn://%s", c.name)
}

func sizeString(s int) string {
	if s >= 1048576 {
		return fmt.Sprintf("%gM", float64(s)/1048576)
	}
	if s >= 1024 {
		return fmt.Sprintf("%gK", float64(s)/1024)
	}
	return fmt.Sprint(s)
}
