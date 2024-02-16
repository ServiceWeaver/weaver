// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package bank includes an example of how to test Service Weaver applications
// using the sim package.
//
// The example involves a bank that incorrectly implements withdrawals.
// Specifically, the withdrawal operation (1) reads a user's bank account
// balance to determine if there are sufficient funds for the withdrawal and
// then (2) performs the actual withdrawal. However, steps (1) and (2) are not
// performed atomically. This lack of atomicity allows two withdrawals to race,
// producing a bank account with a negative balance.
//
// For example, consider a bank account with an initial balance of $100 and two
// operations both racing to withdraw $100. The following execution is possible.
//
//   - Withdraw 1 sees a balance of $100.
//   - Withdraw 2 sees a balance of $100.
//   - Withdraw 2 withdraws, leaving a balance of $0.
//   - Withdraw 1 withdraws, leaving a balance of -$100.
package bank

import (
	"context"
	"fmt"

	"github.com/ServiceWeaver/weaver"
)

// A Bank is a persistent collection of user bank account balances.
type Bank interface {
	// Deposit adds the provided amount to the provided user's bank account
	// balance and returns the balance after the deposit.
	//
	// Deposit returns an error if the provided amount is negative.
	Deposit(ctx context.Context, user string, amount int) (int, error)

	// Withdraw subtracts the provided amount from the provided user's bank
	// account balance and returns the balance after the withdrawal.
	//
	// Withdraw returns an error if the provided amount is negative or if the
	// user's balance is less than the withdrawal amount.
	Withdraw(ctx context.Context, user string, amount int) (int, error)
}

type bank struct {
	weaver.Implements[Bank]
	store weaver.Ref[Store]
}

// Deposit implements the Bank interface.
func (b *bank) Deposit(ctx context.Context, user string, amount int) (int, error) {
	if amount < 0 {
		return 0, fmt.Errorf("deposit negative amount: %d", amount)
	}
	return b.store.Get().Add(ctx, user, amount)
}

// Withdraw implements the Bank interface.
func (b *bank) Withdraw(ctx context.Context, user string, amount int) (int, error) {
	if amount < 0 {
		return 0, fmt.Errorf("withdraw negative amount: %d", amount)
	}
	balance, err := b.store.Get().Get(ctx, user)
	if err != nil {
		return 0, err
	}
	if amount > balance {
		return 0, fmt.Errorf("insufficient funds (%d) to withdraw %d", balance, amount)
	}
	return b.store.Get().Add(ctx, user, -amount)
}
