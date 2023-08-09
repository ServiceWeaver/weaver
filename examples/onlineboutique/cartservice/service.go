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

package cartservice

import (
	"context"
	"errors"

	"github.com/ServiceWeaver/weaver"
)

type CartItem struct {
	weaver.AutoMarshal
	ProductID string
	Quantity  int32
}

type T interface {
	AddItem(ctx context.Context, userID string, item CartItem) error
	GetCart(ctx context.Context, userID string) ([]CartItem, error)
	EmptyCart(ctx context.Context, userID string) error
}

type impl struct {
	weaver.Implements[T]
	cache weaver.Ref[cartCache]
}

// AddItem adds a given item to the user's cart.
func (s *impl) AddItem(ctx context.Context, userID string, item CartItem) error {
	s.Logger(ctx).Info("AddItem called", "userID", userID, "productID", item.ProductID, "quantity", item.Quantity)
	// Get the cart from the cache.
	cart, err := s.cache.Get().Get(ctx, userID)
	if err != nil {
		if errors.Is(err, errNotFound{}) { // cache miss
			cart = nil
		} else {
			return err
		}
	}

	// Copy the cart since the cache may be local, and we don't want to
	// overwrite the cache value directly.
	copy := make([]CartItem, 0, len(cart)+1)
	found := false
	for _, x := range cart {
		if x.ProductID == item.ProductID {
			x.Quantity += item.Quantity
			found = true
		}
		copy = append(copy, x)
	}
	if !found {
		copy = append(copy, CartItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		})
	}

	return s.cache.Get().Add(ctx, userID, copy)
}

// GetCart returns the items in the user's cart.
func (s *impl) GetCart(ctx context.Context, userID string) ([]CartItem, error) {
	s.Logger(ctx).Info("GetCart called", "userID", userID)
	cart, err := s.cache.Get().Get(ctx, userID)
	if err != nil && errors.Is(err, errNotFound{}) {
		return []CartItem{}, nil
	}
	return cart, err
}

// EmptyCart empties the user's cart.
func (s *impl) EmptyCart(ctx context.Context, userID string) error {
	s.Logger(ctx).Info("EmptyCart called", "userID", userID)
	_, err := s.cache.Get().Remove(ctx, userID)
	return err
}
