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

package paymentservice

import (
	"context"
	"time"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/types/money"
)

type CreditCardInfo struct {
	weaver.AutoMarshal
	Number          string
	CVV             int32
	ExpirationYear  int
	ExpirationMonth time.Month
}

// LastFour returns the last four digits of the card number.
func (c CreditCardInfo) LastFour() string {
	num := c.Number
	if len(num) > 4 {
		num = num[len(num)-4:]
	}
	return num
}

type T interface {
	Charge(ctx context.Context, amount money.T, card CreditCardInfo) (string, error)
}

type impl struct {
	weaver.Implements[T]
}

// Charge charges the given amount of money to the given credit card, returning
// the transaction id.
func (s *impl) Charge(ctx context.Context, amount money.T, card CreditCardInfo) (string, error) {
	return charge(amount, card, s.Logger())
}
