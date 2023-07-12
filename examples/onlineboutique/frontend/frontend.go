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
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/adservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/cartservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/checkoutservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/currencyservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/productcatalogservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/recommendationservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/shippingservice"
)

const (
	cookieMaxAge = 60 * 60 * 48

	cookiePrefix    = "shop_"
	cookieSessionID = cookiePrefix + "session-id"
	cookieCurrency  = cookiePrefix + "currency"
)

var (
	//go:embed static/*
	staticFS embed.FS

	validEnvs = []string{"local", "gcp"}

	addrMu    sync.Mutex
	localAddr string
)

type platformDetails struct {
	css      string
	provider string
}

func (plat *platformDetails) setPlatformDetails(env string) {
	if env == "gcp" {
		plat.provider = "Google Cloud"
		plat.css = "gcp-platform"
	} else {
		plat.provider = "local"
		plat.css = "local"
	}
}

// Server is the application frontend.
type Server struct {
	weaver.Implements[weaver.Main]

	handler  http.Handler
	platform platformDetails
	hostname string

	catalogService        weaver.Ref[productcatalogservice.T]
	currencyService       weaver.Ref[currencyservice.T]
	cartService           weaver.Ref[cartservice.T]
	recommendationService weaver.Ref[recommendationservice.T]
	checkoutService       weaver.Ref[checkoutservice.T]
	shippingService       weaver.Ref[shippingservice.T]
	adService             weaver.Ref[adservice.T]

	boutique weaver.Listener
}

func Serve(ctx context.Context, s *Server) error {
	// Find out where we're running.
	// Set ENV_PLATFORM (default to local if not set; use env var if set;
	// otherwise detect GCP, which overrides env).
	var env = os.Getenv("ENV_PLATFORM")
	// Only override from env variable if set + valid env
	if env == "" || !stringinSlice(validEnvs, env) {
		fmt.Println("env platform is either empty or invalid")
		env = "local"
	}
	// Autodetect GCP
	addrs, err := net.LookupHost("metadata.google.internal.")
	if err == nil && len(addrs) >= 0 {
		s.Logger().Debug("Detected Google metadata server, setting ENV_PLATFORM to GCP.", "address", addrs)
		env = "gcp"
	}
	s.Logger().Debug("ENV_PLATFORM", "platform", env)
	s.platform = platformDetails{}
	s.platform.setPlatformDetails(strings.ToLower(env))
	s.hostname, err = os.Hostname()
	if err != nil {
		s.Logger().Debug(`cannot get hostname for frontend: using "unknown"`)
		s.hostname = "unknown"
	}

	// Setup the handler.
	staticHTML, err := fs.Sub(fs.FS(staticFS), "static")
	if err != nil {
		return err
	}
	r := http.NewServeMux()

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
	r.Handle("/", instrument("home", s.homeHandler, []string{get, head}))
	r.Handle("/product/", instrument("product", s.productHandler, []string{get, head}))
	r.Handle("/cart", instrument("cart", s.cartHandler, []string{get, head, post}))
	r.Handle("/cart/empty", instrument("cart_empty", s.emptyCartHandler, []string{post}))
	r.Handle("/setCurrency", instrument("setcurrency", s.setCurrencyHandler, []string{post}))
	r.Handle("/logout", instrument("logout", s.logoutHandler, []string{get}))
	r.Handle("/cart/checkout", instrument("cart_checkout", s.placeOrderHandler, []string{post}))
	r.Handle("/static/", weaver.InstrumentHandler("static", http.StripPrefix("/static/", http.FileServer(http.FS(staticHTML)))))
	r.Handle("/robots.txt", instrument("robots", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "User-agent: *\nDisallow: /") }, nil))
	r.HandleFunc(weaver.HealthzURL, weaver.HealthzHandler)

	// Set handler.
	var handler http.Handler = r
	// TODO(spetrovic): Use the Service Weaver per-component config to provisionaly
	// add these stats.
	handler = ensureSessionID(handler)           // add session ID
	handler = newLogHandler(s.Logger(), handler) // add logging
	s.handler = handler

	s.Logger().Debug("Frontend available", "addr", s.boutique)
	return http.Serve(s.boutique, s.handler)
}
