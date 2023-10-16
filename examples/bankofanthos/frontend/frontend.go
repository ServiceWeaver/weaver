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

package frontend

import (
	"context"
	"crypto/rsa"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/balancereader"
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/contacts"
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/ledgerwriter"
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/transactionhistory"
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/userservice"
	"github.com/golang-jwt/jwt"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/exp/slices"
)

var (
	//go:embed static/*
	staticFS embed.FS

	validEnvs = []string{"local", "gcp"}
)

// serverConfig contains configuration options for the server.
type serverConfig struct {
	publicKey       *rsa.PublicKey
	localRoutingNum string
	bankName        string
	backendTimeout  time.Duration
	clusterName     string
	podName         string
	podZone         string
}

// config contains configuration options read from a weaver TOML file.
type config struct {
	PublicKeyPath         string `toml:"public_key_path"`
	LocalRoutingNum       string `toml:"local_routing_num"`
	BankName              string `toml:"bank_name"`
	BackendTimeoutSeconds int    `toml:"backend_timeout_seconds"`
}

// server is the application frontend.
type server struct {
	weaver.Implements[weaver.Main]
	weaver.WithConfig[config]
	lis                weaver.Listener `weaver:"bank"`
	balanceReader      weaver.Ref[balancereader.T]
	contacts           weaver.Ref[contacts.T]
	ledgerWriter       weaver.Ref[ledgerwriter.T]
	transactionHistory weaver.Ref[transactionhistory.T]
	userService        weaver.Ref[userservice.T]

	hostname string
	config   serverConfig
}

// Init initializes an application frontend.
func (s *server) Init(ctx context.Context) error {
	// Find out where we're running.
	var env = os.Getenv("ENV_PLATFORM")
	// Only override from env variable if set + valid env.
	if env == "" || !slices.Contains(validEnvs, env) {
		s.Logger(ctx).Debug("ENV_PLATFORM is either empty or invalid")
		env = "local"
	}
	// Autodetect GCP.
	var err error
	addrs, err := net.LookupHost("metadata.google.internal.")
	if err == nil && len(addrs) > 0 {
		s.Logger(ctx).Debug("Detected Google metadata server, setting ENV_PLATFORM to GCP.", "address", addrs)
		env = "gcp"
	}
	s.Logger(ctx).Debug("ENV_PLATFORM", "platform", env)

	s.hostname, err = os.Hostname()
	if err != nil {
		s.Logger(ctx).Debug(`cannot get hostname for frontend: using "unknown"`)
		s.hostname = "unknown"
	}

	metadataServer := os.Getenv("METADATA_SERVER")
	if metadataServer == "" {
		metadataServer = "metadata.google.internal"
	}
	metadataURL := fmt.Sprintf("http://%s/computeMetadata/v1/", metadataServer)
	metadataHeaders := http.Header{}
	metadataHeaders.Set("Metadata-Flavor", "Google")
	s.config.clusterName = getClusterName(metadataURL, metadataHeaders)
	s.config.podName = s.hostname
	s.config.podZone = getPodZone(metadataURL, metadataHeaders)

	pubKeyBytes, err := os.ReadFile(s.Config().PublicKeyPath)
	if err != nil {
		return fmt.Errorf("unable to read public key file: %w", err)
	}
	s.config.publicKey, err = jwt.ParseRSAPublicKeyFromPEM(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("unable to parse public key: %w", err)
	}
	s.config.localRoutingNum = s.Config().LocalRoutingNum
	s.config.backendTimeout = time.Duration(s.Config().BackendTimeoutSeconds) * time.Second
	s.config.bankName = s.Config().BankName
	return nil
}

// Serve runs the Bank of Anthos server.
func Serve(ctx context.Context, s *server) error {
	// Setup the handler.
	staticHTML, err := fs.Sub(fs.FS(staticFS), "static")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()

	// Helper that adds a handler with HTTP metric instrumentation.
	instrument := func(label string, fn func(http.ResponseWriter, *http.Request), methods []string) http.Handler {
		allowed := map[string]struct{}{}
		for _, method := range methods {
			allowed[method] = struct{}{}
		}
		handler := func(w http.ResponseWriter, r *http.Request) {
			if _, ok := allowed[r.Method]; len(allowed) > 0 && !ok {
				msg := fmt.Sprintf("method %q not allowed", r.Method)
				http.Error(w, msg, http.StatusMethodNotAllowed)
				return
			}
			fn(w, r)
		}
		return weaver.InstrumentHandlerFunc(label, handler)
	}

	const get = http.MethodGet
	const post = http.MethodPost
	const head = http.MethodHead
	mux.Handle("/", instrument("root", s.rootHandler, []string{get, head}))
	mux.Handle("/home/", instrument("home", s.homeHandler, []string{get, head}))
	mux.Handle("/payment", instrument("payment", s.paymentHandler, []string{post}))
	mux.Handle("/deposit", instrument("deposit", s.depositHandler, []string{post}))
	mux.Handle("/login", instrument("login", s.loginHandler, []string{get, post}))
	mux.Handle("/consent", instrument("consent", s.consentHandler, []string{get, post}))
	mux.Handle("/signup", instrument("signup", s.signupHandler, []string{get, post}))
	mux.Handle("/logout", instrument("logout", s.logoutHandler, []string{post}))
	mux.Handle("/static/", weaver.InstrumentHandler("static", http.StripPrefix("/static", http.FileServer(http.FS(staticHTML)))))

	// No instrumentation of /healthz
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "ok") })

	// Set handler and return.
	var handler http.Handler = mux
	handler = newLogHandler(s, handler)            // add logging
	handler = otelhttp.NewHandler(handler, "http") // add tracing
	s.Logger(ctx).Debug("Frontend available", "addr", s.lis)
	return http.Serve(s.lis, handler)
}

func getClusterName(metadataURL string, metadataHeaders http.Header) string {
	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "unknown"
	}
	req, err := http.NewRequest("GET", metadataURL+"instance/attributes/cluster-name", nil)
	if err != nil {
		return clusterName
	}
	req.Header = metadataHeaders

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return clusterName
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return clusterName
	}
	
	clusterNameBytes := make([]byte, resp.ContentLength)
	_, err = resp.Body.Read(clusterNameBytes)
	if err != nil {
		return clusterName
	}
	clusterName = string(clusterNameBytes)
	return clusterName
}

func getPodZone(metadataURL string, metadataHeaders http.Header) string {
	podZone := os.Getenv("POD_ZONE")
	if podZone == "" {
		podZone = "unknown"
	}
	req, err := http.NewRequest("GET", metadataURL+"instance/zone", nil)
	if err != nil {
		return podZone
	}
	req.Header = metadataHeaders

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return podZone
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		podZoneBytes := make([]byte, resp.ContentLength)
		_, err = resp.Body.Read(podZoneBytes)
		if err != nil {
			return podZone
		}
		podZoneSplit := strings.Split(string(podZoneBytes), "/")
		if len(podZoneSplit) >= 4 {
			podZone = podZoneSplit[3]
		}
	}
	return podZone
}
