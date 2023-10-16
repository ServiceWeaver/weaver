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

package ledgerwriter

import (
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type transactionRepository struct {
	db *gorm.DB
}

func newTransactionRepository(databaseURI string) (*transactionRepository, error) {
	db, err := gorm.Open(postgres.Open(databaseURI))
	if err != nil {
		return nil, err
	}
	return &transactionRepository{db: db}, nil
}

func (r *transactionRepository) save(transaction *model.Transaction) error {
	return r.db.Create(transaction).Error
}
