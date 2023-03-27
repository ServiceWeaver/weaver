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
	"fmt"
	"strings"
	"time"

	"github.com/ServiceWeaver/weaver/examples/onlineboutique/types/money"
	"github.com/google/uuid"
	"golang.org/x/exp/slog"
)

type InvalidCreditCardErr struct{}

func (e InvalidCreditCardErr) Error() string {
	return "invalid credit card"
}

type UnacceptedCreditCardErr struct{}

func (e UnacceptedCreditCardErr) Error() string {
	return "credit card not accepted; only VISA or MasterCard are accepted"
}

type ExpiredCreditCardErr struct{}

func (e ExpiredCreditCardErr) Error() string {
	return "credit card expired"
}

func charge(amount money.T, card CreditCardInfo, logger *slog.Logger) (string, error) {
	// Perform some rudimentary validation.
	number := strings.ReplaceAll(card.Number, "-", "")
	var company string
	switch {
	case len(number) < 4:
		return "", InvalidCreditCardErr{}
	case number[0] == '4':
		company = "Visa"
	case number[0] == '5':
		company = "MasterCard"
	default:
		return "", InvalidCreditCardErr{}
	}
	if card.CVV < 100 || card.CVV > 9999 {
		return "", InvalidCreditCardErr{}
	}
	if time.Date(card.ExpirationYear, card.ExpirationMonth, 0, 0, 0, 0, 0, time.Local).Before(time.Now()) {
		return "", ExpiredCreditCardErr{}
	}

	// Card is valid: process the transaction.
	logger.Info(
		"Transaction processed",
		"company", company,
		"last_four", number[len(number)-4:],
		"currency", amount.CurrencyCode,
		"amount", fmt.Sprintf("%d.%d", amount.Units, amount.Nanos),
	)
	return uuid.New().String(), nil
}
