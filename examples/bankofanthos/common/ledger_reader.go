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

package common

import (
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/ServiceWeaver/weaver/examples/bankofanthos/model"
)

const (
	startingTransactionID int64 = -1
)

// LedgerReaderCallback provides a callback method invoked by ledger reader on
// discovering new transactions.
type LedgerReaderCallback interface {
	// ProcessTransaction processes a newly discovered transaction. For example,
	// an implementation might update it's cache based on the contents of the
	// new transaction.
	ProcessTransaction(transaction model.Transaction)
}

// LedgerReader listens for and reacts to incoming transactions.
type LedgerReader struct {
	dbRepo               ReadOnlyTransactionRepository
	pollMs               int
	callback             LedgerReaderCallback
	latestTransactionID  int64
	backgroundThreadDead atomic.Bool
	logger               *slog.Logger
}

// NewLedgerReader returns a ledger reader over a transaction repository.
func NewLedgerReader(dbRepo ReadOnlyTransactionRepository, logger *slog.Logger) *LedgerReader {
	return &LedgerReader{
		dbRepo: dbRepo,
		pollMs: 100,
		logger: logger,
	}
}

// StartWithCallback synchronously loads all existing transactions, and then starts
// a background thread to listen for future transactions.
func (reader *LedgerReader) StartWithCallback(callback LedgerReaderCallback) error {
	if callback == nil {
		reader.logger.Info("Callback is nil")
		return fmt.Errorf("callback is nil")
	}
	reader.callback = callback
	reader.latestTransactionID = startingTransactionID
	// Get the latest transaction id in ledger.
	var err error
	reader.latestTransactionID, err = reader.getLatestTransactionID()
	if err != nil {
		reader.logger.Debug("Could not contact ledger database at init")
	}
	// Start a background thread to listen for future transactions.
	go reader.backgroundThreadFunc()
	return nil
}

func (reader *LedgerReader) backgroundThreadFunc() {
	alive := true
	for alive {
		// Sleep between polls.
		time.Sleep(time.Duration(reader.pollMs) * time.Millisecond)
		// Check for new transactions in the ledger database.
		remoteLatest, err := reader.getLatestTransactionID()
		if err != nil {
			remoteLatest = reader.latestTransactionID
			reader.logger.Debug("Could not reach ledger database: ", "error", err)
		}
		// If there are new transactions, poll the database.
		if remoteLatest > reader.latestTransactionID {
			reader.latestTransactionID = reader.pollTransactions(reader.latestTransactionID)
		} else if remoteLatest < reader.latestTransactionID {
			// Remote database is out of sync. Suspend processing transactions to reset the service.
			alive = false
			reader.logger.Debug("Remote transaction id out of sync")
		}
	}
	reader.backgroundThreadDead.Store(true)
}

// pollTransactions polls for new transactions and invokes processTransaction
// callback for each new transaction.
func (reader *LedgerReader) pollTransactions(startingID int64) int64 {
	latestID := startingID
	transactionList, err := reader.dbRepo.FindLatest(startingID)
	if err != nil {
		reader.logger.Error("Error polling transactions", err)
		return latestID
	}
	reader.logger.Info("Polling new transactions")
	for _, transaction := range transactionList {
		reader.callback.ProcessTransaction(transaction.Transaction)
		latestID = transaction.TransactionID
	}
	return latestID
}

// IsAlive returns true if the ledger reader is alive and polling for new transactions.
func (reader *LedgerReader) IsAlive() bool {
	return !reader.backgroundThreadDead.Load()
}

func (reader *LedgerReader) getLatestTransactionID() (int64, error) {
	latestID, err := reader.dbRepo.LatestTransactionID()
	if err != nil {
		return startingTransactionID, err
	}
	return latestID, nil
}
