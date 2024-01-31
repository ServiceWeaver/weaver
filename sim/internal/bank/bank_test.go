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

package bank_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/sim"
	"github.com/ServiceWeaver/weaver/sim/internal/bank"
)

// fakestore is a fake implementation of the Store component that uses an
// in-memory map[string]int to store values.
type fakestore struct {
	mu     sync.Mutex
	values map[string]int
}

var _ bank.Store = (*fakestore)(nil)

// Get implements the Store interface.
func (f *fakestore) Get(_ context.Context, key string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.values[key], nil
}

// Add implements the Store interface.
func (f *fakestore) Add(_ context.Context, key string, delta int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[key] += delta
	return f.values[key], nil
}

// BankWorkload is a workload that performs random deposits and withdrawals. It
// checks the invariant that bank account balances can never be negative.
type BankWorkload struct {
	bank weaver.Ref[bank.Bank]
}

// Init implements the sim.Workload interface.
func (c *BankWorkload) Init(r sim.Registrar) error {
	// Register generators that deposit and withdraw between $0 and $100 from
	// alice and bob's bank accounts.
	user := sim.OneOf("alice", "bob")
	amount := sim.Range(0, 100)
	r.RegisterGenerators("Deposit", user, amount)
	r.RegisterGenerators("Withdraw", user, amount)

	// Register a fake store with $100 in alice and bob's accounts initially.
	store := &fakestore{values: map[string]int{"alice": 100, "bob": 100}}
	r.RegisterFake(sim.Fake[bank.Store](store))
	return nil
}

// Deposit is an operation that deposits the provided amount in the provided
// user's bank account balance.
func (c *BankWorkload) Deposit(ctx context.Context, user string, amount int) error {
	// NOTE that we ignore errors because it is expected that the simulator
	// will inject errors every once in a while.
	c.bank.Get().Deposit(ctx, user, amount)
	return nil
}

// Withdraw is an operation that withdraws the provided amount from the
// provided user's bank account balance.
func (c *BankWorkload) Withdraw(ctx context.Context, user string, amount int) error {
	balance, err := c.bank.Get().Withdraw(ctx, user, amount)
	if err != nil {
		// NOTE that we ignore errors because it is expected that the simulator
		// will inject errors every once in a while.
		return nil
	}
	if balance < 0 {
		// A bank account balance should never be negative.
		return fmt.Errorf("user %s has negative balance %d", user, balance)
	}
	return nil
}

func TestBank(t *testing.T) {
	s := sim.New(t, &BankWorkload{}, sim.Options{})
	r := s.Run(10 * time.Second)
	if r.Err == nil {
		t.Fatal("Unexpected success")
	}
	t.Log(r.Mermaid())
}
