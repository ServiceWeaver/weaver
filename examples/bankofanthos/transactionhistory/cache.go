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
	"time"

	cache "github.com/goburrow/cache"
)

// transactionCache maintains a cache of transaction retrieved from the ledger database.
type transactionCache struct {
	c cache.LoadingCache
}

func newTransactionCache(txnRepo *transactionRepository, cacheSize int, expireMinutes int, localRoutingNum string, historyLimit int) *transactionCache {
	load := func(accountID cache.Key) (cache.Value, error) {
		return txnRepo.findForAccount(accountID.(string), localRoutingNum, historyLimit)
	}

	return &transactionCache{
		c: cache.NewLoadingCache(
			load,
			cache.WithMaximumSize(cacheSize),
			cache.WithExpireAfterWrite(time.Duration(expireMinutes)*time.Minute),
		),
	}
}
