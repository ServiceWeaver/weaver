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
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/common"
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/model"
)

// transactionRepository is a repository for performing queries on the ledger database.
type transactionRepository struct {
	common.LedgerReaderTransactionRepository
}

func newTransactionRepository(databaseURI string) (*transactionRepository, error) {
	repo, err := common.NewLedgerReaderTransactionRepository(databaseURI)
	if err != nil {
		return nil, err
	}
	return &transactionRepository{LedgerReaderTransactionRepository: *repo}, nil
}

// findForAccount returns a list of transactions for the given account and routing numbers.
func (r *transactionRepository) findForAccount(accountNum string, routingNum string, limit int) ([]model.Transaction, error) {
	var txns []model.Transaction
	result := r.DB.Table("transactions").Where("(from_acct=? AND from_route=?) OR (to_acct=? AND to_route=?)", accountNum, routingNum, accountNum, routingNum).Order("timestamp DESC").Find(&txns)
	if result.Error != nil {
		return nil, result.Error
	}
	return txns, nil
}
