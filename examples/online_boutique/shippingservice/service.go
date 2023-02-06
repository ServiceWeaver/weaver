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

package shippingservice

import (
	"context"
	"fmt"

	weaver "github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/online_boutique/cartservice"
	"github.com/ServiceWeaver/weaver/examples/online_boutique/types/money"
)

type Address struct {
	weaver.AutoMarshal
	StreetAddress string
	City          string
	State         string
	Country       string
	ZipCode       int32
}

type T interface {
	GetQuote(ctx context.Context, addr Address, items []cartservice.CartItem) (money.T, error)
	ShipOrder(ctx context.Context, addr Address, items []cartservice.CartItem) (string, error)
}

type impl struct {
	weaver.Implements[T]
}

// GetQuote produces a shipping quote (cost) in USD.
func (s *impl) GetQuote(ctx context.Context, addr Address, items []cartservice.CartItem) (money.T, error) {
	s.Logger().Info("[GetQuote] received request")
	defer s.Logger().Info("[GetQuote] completed request")

	// 1. Generate a quote based on the total number of items to be shipped.
	quote := createQuoteFromCount(len(items))

	// 2. Generate a response.
	return money.T{
		CurrencyCode: "USD",
		Units:        int64(quote.Dollars),
		Nanos:        int32(quote.Cents * 10000000),
	}, nil
}

// ShipOrder mocks that the requested items will be shipped.
// It supplies a tracking ID for notional lookup of shipment delivery status.
func (s *impl) ShipOrder(ctx context.Context, addr Address, items []cartservice.CartItem) (string, error) {
	s.Logger().Info("[ShipOrder] received request")
	defer s.Logger().Info("[ShipOrder] completed request")

	// Create a Tracking ID.
	baseAddress := fmt.Sprintf("%s, %s, %s", addr.StreetAddress, addr.City, addr.State)
	id := createTrackingID(baseAddress)
	return id, nil
}
