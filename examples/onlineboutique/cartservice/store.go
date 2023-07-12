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

	"golang.org/x/exp/slog"
)

type cartStore struct {
	logger *slog.Logger
	cache  cartCache
}

func newCartStore(logger *slog.Logger, cache cartCache) (*cartStore, error) {
	return &cartStore{logger: logger, cache: cache}, nil
}

func (c *cartStore) AddItem(ctx context.Context, userID, productID string, quantity int32) error {
	c.logger.Info("AddItem called", "userID", userID, "productID", productID, "quantity", quantity)
	// Get the cart from the cache.
	cart, err := c.cache.Get(ctx, userID)
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
	for _, item := range cart {
		if item.ProductID == productID {
			item.Quantity += quantity
			found = true
		}
		copy = append(copy, item)
	}
	if !found {
		copy = append(copy, CartItem{
			ProductID: productID,
			Quantity:  quantity,
		})
	}

	return c.cache.Add(ctx, userID, copy)
}

func (c *cartStore) EmptyCart(ctx context.Context, userID string) error {
	c.logger.Info("EmptyCart called", "userID", userID)
	_, err := c.cache.Remove(ctx, userID)
	return err
}

func (c *cartStore) GetCart(ctx context.Context, userID string) ([]CartItem, error) {
	c.logger.Info("GetCart called", "userID", userID)
	cart, err := c.cache.Get(ctx, userID)
	if err != nil && errors.Is(err, errNotFound{}) {
		return []CartItem{}, nil
	}
	return cart, err
}
