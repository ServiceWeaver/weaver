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

package balancereader

import (
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/common"
	_ "github.com/jinzhu/gorm/dialects/postgres"
)

// TransactionRepository is a repository for performing queries on the Transaction database.
type TransactionRepository struct {
	common.LedgerReaderTransactionRepository
}

func newTransactionRepository(databaseURI string) (*TransactionRepository, error) {
	repo, err := common.NewLedgerReaderTransactionRepository(databaseURI)
	if err != nil {
		return nil, err
	}
	return &TransactionRepository{LedgerReaderTransactionRepository: *repo}, nil
}

// findBalance returns the current balance of the given account.
func (r *TransactionRepository) findBalance(accountNum, routingNum string) (int64, error) {
	sql := `
SELECT
  (
    SELECT SUM(AMOUNT) FROM TRANSACTIONS t
    WHERE (TO_ACCT = ? AND TO_ROUTE = ?)
  )
  -
  (
    SELECT COALESCE((SELECT SUM(AMOUNT) FROM TRANSACTIONS t
    WHERE (FROM_ACCT = ? AND FROM_ROUTE = ?)),0)
  )
  as balance
`
	var balance int64
	row := r.DB.Raw(sql, accountNum, routingNum, accountNum, routingNum).Row()
	row.Scan(&balance)
	return balance, nil
}
