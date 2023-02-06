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

	creditcard "github.com/durango/go-credit-card"
	"github.com/google/uuid"
	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/online_boutique/types/money"
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

func charge(amount money.T, card CreditCardInfo, logger weaver.Logger) (string, error) {
	// Validate the card using the validation library.
	c := creditcard.Card{
		// NOTE: creditcard library doesn't support dashes in credit-card
		// numbers: remove them.
		Number: strings.ReplaceAll(card.Number, "-", ""),
		Cvv:    fmt.Sprint(card.CVV),
		Month:  fmt.Sprint(card.ExpirationMonth),
		Year:   fmt.Sprint(card.ExpirationYear),
	}
	if err := c.Method(); err != nil {
		return "", InvalidCreditCardErr{}
	}
	if c.Company.Short != "visa" && c.Company.Short != "mastercard" {
		return "", UnacceptedCreditCardErr{}
	}
	if err := c.ValidateCVV(); err != nil {
		return "", InvalidCreditCardErr{}
	}
	if ok := c.ValidateNumber(); !ok {
		return "", InvalidCreditCardErr{}
	}
	if err := c.ValidateExpiration(); err != nil {
		if strings.Contains(err.Error(), "expired") {
			return "", ExpiredCreditCardErr{}
		}
		return "", InvalidCreditCardErr{}
	}

	// Card is valid: process the transaction.
	logger.Info(
		"Transaction processed",
		"company", c.Company.Long,
		"last_four", card.LastFour(),
		"currency", amount.CurrencyCode,
		"amount", fmt.Sprintf("%d.%d", amount.Units, amount.Nanos),
	)
	return uuid.New().String(), nil
}
