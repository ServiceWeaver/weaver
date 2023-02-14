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

package checkoutservice

import (
	"context"
	"fmt"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/cartservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/currencyservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/emailservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/paymentservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/productcatalogservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/shippingservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/types"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/types/money"
	"github.com/google/uuid"
)

type PlaceOrderRequest struct {
	weaver.AutoMarshal
	UserID       string
	UserCurrency string
	Address      shippingservice.Address
	Email        string
	CreditCard   paymentservice.CreditCardInfo
}

type T interface {
	PlaceOrder(ctx context.Context, req PlaceOrderRequest) (types.Order, error)
}

type impl struct {
	weaver.Implements[T]

	catalogService  productcatalogservice.T
	cartService     cartservice.T
	currencyService currencyservice.T
	shippingService shippingservice.T
	emailService    emailservice.T
	paymentService  paymentservice.T
}

func (s *impl) Init(context.Context) error {
	var err error
	s.catalogService, err = weaver.Get[productcatalogservice.T](s)
	if err != nil {
		return err
	}
	s.cartService, err = weaver.Get[cartservice.T](s)
	if err != nil {
		return err
	}
	s.currencyService, err = weaver.Get[currencyservice.T](s)
	if err != nil {
		return err
	}
	s.shippingService, err = weaver.Get[shippingservice.T](s)
	if err != nil {
		return err
	}
	s.emailService, err = weaver.Get[emailservice.T](s)
	if err != nil {
		return err
	}
	s.paymentService, err = weaver.Get[paymentservice.T](s)
	if err != nil {
		return err
	}
	return nil
}

func (s *impl) PlaceOrder(ctx context.Context, req PlaceOrderRequest) (types.Order, error) {
	s.Logger().Info("[PlaceOrder]", "user_id", req.UserID, "user_currency", req.UserCurrency)

	prep, err := s.prepareOrderItemsAndShippingQuoteFromCart(ctx, req.UserID, req.UserCurrency, req.Address)
	if err != nil {
		return types.Order{}, err
	}

	total := money.T{
		CurrencyCode: req.UserCurrency,
		Units:        0,
		Nanos:        0,
	}
	total = money.Must(money.Sum(total, prep.shippingCostLocalized))
	for _, it := range prep.orderItems {
		multPrice := money.MultiplySlow(it.Cost, uint32(it.Item.Quantity))
		total = money.Must(money.Sum(total, multPrice))
	}

	txID, err := s.paymentService.Charge(ctx, total, req.CreditCard)
	if err != nil {
		return types.Order{}, fmt.Errorf("failed to charge card: %w", err)
	}
	s.Logger().Info("payment went through", "transaction_id", txID)

	shippingTrackingID, err := s.shippingService.ShipOrder(ctx, req.Address, prep.cartItems)
	if err != nil {
		return types.Order{}, fmt.Errorf("shipping error: %w", err)
	}

	_ = s.cartService.EmptyCart(ctx, req.UserID)

	order := types.Order{
		OrderID:            uuid.New().String(),
		ShippingTrackingID: shippingTrackingID,
		ShippingCost:       prep.shippingCostLocalized,
		ShippingAddress:    req.Address,
		Items:              prep.orderItems,
	}

	if err := s.emailService.SendOrderConfirmation(ctx, req.Email, order); err != nil {
		s.Logger().Error("failed to send order confirmation", err, "email", req.Email)
	} else {
		s.Logger().Info("order confirmation email sent", "email", req.Email)
	}
	return order, nil
}

type orderPrep struct {
	orderItems            []types.OrderItem
	cartItems             []cartservice.CartItem
	shippingCostLocalized money.T
}

func (s *impl) prepareOrderItemsAndShippingQuoteFromCart(ctx context.Context, userID, userCurrency string, address shippingservice.Address) (orderPrep, error) {
	var out orderPrep
	cartItems, err := s.cartService.GetCart(ctx, userID)
	if err != nil {
		return out, fmt.Errorf("failed to get user cart during checkout: %w", err)
	}
	orderItems, err := s.prepOrderItems(ctx, cartItems, userCurrency)
	if err != nil {
		return out, fmt.Errorf("failed to prepare order: %w", err)
	}
	shippingUSD, err := s.shippingService.GetQuote(ctx, address, cartItems)
	if err != nil {
		return out, fmt.Errorf("failed to get shipping quote: %w", err)
	}
	shippingPrice, err := s.currencyService.Convert(ctx, shippingUSD, userCurrency)
	if err != nil {
		return out, fmt.Errorf("failed to convert shipping cost to currency: %w", err)
	}

	out.shippingCostLocalized = shippingPrice
	out.cartItems = cartItems
	out.orderItems = orderItems
	return out, nil
}

func (s *impl) prepOrderItems(ctx context.Context, items []cartservice.CartItem, userCurrency string) ([]types.OrderItem, error) {
	out := make([]types.OrderItem, len(items))
	for i, item := range items {
		product, err := s.catalogService.GetProduct(ctx, item.ProductID)
		if err != nil {
			return nil, fmt.Errorf("failed to get product #%q: %w", item.ProductID, err)
		}
		price, err := s.currencyService.Convert(ctx, product.PriceUSD, userCurrency)
		if err != nil {
			return nil, fmt.Errorf("failed to convert price of %q to %s: %w", item.ProductID, userCurrency, err)
		}
		out[i] = types.OrderItem{
			Item: item,
			Cost: price,
		}
	}
	return out, nil
}
