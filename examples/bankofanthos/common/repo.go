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
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ReadOnlyTransactionRepository is a read-only interface over transaction repository.
type ReadOnlyTransactionRepository interface {
	// LatestTransactionID returns the latest transaction id from the repository.
	LatestTransactionID() (int64, error)
	// FindLatest returns transactions that are more recent than and thus have an
	// id greater than startTransactionID.
	FindLatest(startTransactionID int64) ([]model.TransactionWithID, error)
}

// LedgerReaderTransactionRepository exposes methods used by LedgerReader.
type LedgerReaderTransactionRepository struct {
	DB *gorm.DB
}

// NewLedgerReaderTransactionRepository returns a new repository over a database.
func NewLedgerReaderTransactionRepository(databaseURI string) (*LedgerReaderTransactionRepository, error) {
	db, err := gorm.Open(postgres.Open(databaseURI))
	if err != nil {
		return nil, err
	}
	return &LedgerReaderTransactionRepository{DB: db}, nil
}

// LatestTransactionID returns the id of the latest transaction, or NULL if none exist.
func (r *LedgerReaderTransactionRepository) LatestTransactionID() (int64, error) {
	sql := "SELECT MAX(transaction_id) FROM Transactions"
	var maxID int64
	if err := r.DB.Raw(sql).Row().Scan(&maxID); err != nil {
		return 0, err
	}
	return maxID, nil
}

// FindLatest returns all the transaction committed after startID and thus have an id > startID.
func (r *LedgerReaderTransactionRepository) FindLatest(startID int64) ([]model.TransactionWithID, error) {
	var txns []model.TransactionWithID
	result := r.DB.Table("transactions").Where("transaction_id > ?", startID).Order("transaction_id ASC").Find(&txns)
	if result.Error != nil {
		return nil, result.Error
	}
	return txns, nil
}
