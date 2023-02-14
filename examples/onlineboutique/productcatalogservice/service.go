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

package productcatalogservice

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/types/money"
)

var (
	//go:embed products.json
	catalogFileData []byte
)

type NotFoundError struct{}

func (e NotFoundError) Error() string { return "product not found" }

type Product struct {
	weaver.AutoMarshal
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Picture     string  `json:"picture"`
	PriceUSD    money.T `json:"priceUsd"`

	// Categories such as "clothing" or "kitchen" that can be used to look up
	// other related products.
	Categories []string `json:"categories"`
}

type T interface {
	ListProducts(ctx context.Context) ([]Product, error)
	GetProduct(ctx context.Context, productID string) (Product, error)
	SearchProducts(ctx context.Context, query string) ([]Product, error)
}

type impl struct {
	weaver.Implements[T]

	extraLatency time.Duration

	mu            sync.RWMutex
	cat           []Product
	reloadCatalog bool
}

func (s *impl) Init(context.Context) error {
	var extraLatency time.Duration
	if extra := os.Getenv("EXTRA_LATENCY"); extra != "" {
		v, err := time.ParseDuration(extra)
		if err != nil {
			return fmt.Errorf("failed to parse EXTRA_LATENCY (%s) as time.Duration: %+v", v, err)
		}
		extraLatency = v
		s.Logger().Info("extra latency enabled", "duration", extraLatency)
	}
	s.extraLatency = extraLatency
	_, err := s.refreshCatalogFile()
	if err != nil {
		return fmt.Errorf("could not parse product catalog: %w", err)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for {
			sig := <-sigs
			s.Logger().Info("Received signal", "signal", sig)
			reload := false
			if sig == syscall.SIGUSR1 {
				reload = true
				s.Logger().Info("Enable catalog reloading")
			} else {
				s.Logger().Info("Disable catalog reloading")
			}
			s.mu.Lock()
			s.reloadCatalog = reload
			s.mu.Unlock()
		}
	}()

	return nil
}

func (s *impl) refreshCatalogFile() ([]Product, error) {
	var products []Product
	if err := json.Unmarshal(catalogFileData, &products); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cat = products
	return products, nil
}

func (s *impl) getCatalogState() (bool, []Product) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reloadCatalog, s.cat
}

func (s *impl) parseCatalog() []Product {
	reload, products := s.getCatalogState()
	if reload || len(products) == 0 {
		var err error
		if products, err = s.refreshCatalogFile(); err != nil {
			products = nil
		}
	}
	return products
}

func (s *impl) ListProducts(ctx context.Context) ([]Product, error) {
	time.Sleep(s.extraLatency)
	return s.parseCatalog(), nil
}

func (s *impl) GetProduct(ctx context.Context, productID string) (Product, error) {
	time.Sleep(s.extraLatency)
	for _, p := range s.parseCatalog() {
		if p.ID == productID {
			return p, nil
		}
	}
	return Product{}, NotFoundError{}
}

func (s *impl) SearchProducts(ctx context.Context, query string) ([]Product, error) {
	time.Sleep(s.extraLatency)

	// Intepret query as a substring match in name or description.
	var ps []Product
	for _, p := range s.parseCatalog() {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(p.Description), strings.ToLower(query)) {
			ps = append(ps, p)
		}
	}
	return ps, nil
}
