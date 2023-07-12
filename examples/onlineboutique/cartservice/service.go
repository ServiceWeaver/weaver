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
	store *cartStore
}

func (s *impl) Init(context.Context) error {
	store, err := newCartStore(s.Logger(), s.cache.Get())
	s.store = store
	return err
}

// AddItem adds a given item to the user's cart.
func (s *impl) AddItem(ctx context.Context, userID string, item CartItem) error {
	return s.store.AddItem(ctx, userID, item.ProductID, item.Quantity)
}

// GetCart returns the items in the user's cart.
func (s *impl) GetCart(ctx context.Context, userID string) ([]CartItem, error) {
	return s.store.GetCart(ctx, userID)
}

// EmptyCart empties the user's cart.
func (s *impl) EmptyCart(ctx context.Context, userID string) error {
	return s.store.EmptyCart(ctx, userID)
}
