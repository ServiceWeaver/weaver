// Copyright 2023 Google LLC
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

package transactionhistory

import (
	"context"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/common"
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/model"
)

type T interface {
	// GetTransactions returns all the transactions of an account.
	GetTransactions(ctx context.Context, accountID string) ([]model.Transaction, error)
}

type config struct {
	LocalRoutingNum string `toml:"local_routing_num"`
	DataSourceURL   string `toml:"data_source_url"`
	HistoryLimit    int    `toml:"history_limit"`
	CacheSize       int    `toml:"cache_size"`
	CacheMinutes    int    `toml:"cache_minutes"`
}

type impl struct {
	weaver.Implements[T]
	weaver.WithConfig[config]

	txnRepo      *transactionRepository
	txnCache     *transactionCache
	ledgerReader *common.LedgerReader
}

var _ common.LedgerReaderCallback = (*impl)(nil)

// ProcessTransaction implements the common.LedgerReaderCallback interface.
func (i *impl) ProcessTransaction(transaction model.Transaction) {
	fromID := transaction.FromAccountNum
	fromRoutingNum := transaction.FromRoutingNum
	toID := transaction.ToAccountNum
	toRouting := transaction.ToRoutingNum
	if fromRoutingNum == i.Config().LocalRoutingNum {
		if _, ok := i.txnCache.c.GetIfPresent(fromID); ok {
			i.processTransactionForAcct(fromID, transaction)
		}
	}
	if toRouting == i.Config().LocalRoutingNum {
		if _, ok := i.txnCache.c.GetIfPresent(toID); ok {
			i.processTransactionForAcct(toID, transaction)
		}
	}
}

func (i *impl) processTransactionForAcct(accountID string, transaction model.Transaction) {
	ctx := context.TODO()
	i.Logger(ctx).Debug("Processing transaction", "accountID", accountID, "transaction", transaction)
	got, err := i.txnCache.c.Get(accountID)
	if err != nil {
		i.Logger(ctx).Error("processTransactionForAcct failed", err)
		return
	}
	txns := got.([]model.Transaction)
	txns = append([]model.Transaction{transaction}, txns...)
	// Drop old transactions.
	if len(txns) > i.Config().HistoryLimit {
		i.Logger(ctx).Debug("Hit transaction caching limit, dropping old transactions", "dropped", len(txns)-i.Config().HistoryLimit)
		txns = txns[:i.Config().HistoryLimit]
	}
	i.txnCache.c.Put(accountID, txns)
}

func (i *impl) Init(ctx context.Context) error {
	var err error
	i.txnRepo, err = newTransactionRepository(i.Config().DataSourceURL)
	if err != nil {
		return err
	}
	i.txnCache = newTransactionCache(i.txnRepo, i.Config().CacheSize, i.Config().CacheMinutes, i.Config().LocalRoutingNum, i.Config().HistoryLimit)
	i.ledgerReader = common.NewLedgerReader(i.txnRepo, i.Logger(ctx))
	i.ledgerReader.StartWithCallback(i)
	return nil
}

func (i *impl) GetTransactions(ctx context.Context, accountID string) ([]model.Transaction, error) {
	// Load from cache.
	got, err := i.txnCache.c.Get(accountID)
	if err != nil {
		return nil, err
	}
	return got.([]model.Transaction), nil
}
