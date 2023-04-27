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

package recommendationservice

import (
	"context"
	"math/rand"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/productcatalogservice"
)

type T interface {
	ListRecommendations(ctx context.Context, userID string, productIDs []string) ([]string, error)
}

type impl struct {
	weaver.Implements[T]
	catalogService weaver.Ref[productcatalogservice.T]
}

func (s *impl) ListRecommendations(ctx context.Context, userID string, userProductIDs []string) ([]string, error) {
	// Fetch a list of products from the product catalog.
	catalogProducts, err := s.catalogService.Get().ListProducts(ctx)
	if err != nil {
		return nil, err
	}

	// Remove user-provided products from the catalog, to avoid recommending
	// them.
	userIDs := make(map[string]struct{}, len(userProductIDs))
	for _, id := range userProductIDs {
		userIDs[id] = struct{}{}
	}
	filtered := make([]string, 0, len(catalogProducts))
	for _, product := range catalogProducts {
		if _, ok := userIDs[product.ID]; ok {
			continue
		}
		filtered = append(filtered, product.ID)
	}

	// Sample from filtered products and return them.
	perm := rand.Perm(len(filtered))
	const maxResponses = 5
	ret := make([]string, 0, maxResponses)
	for _, idx := range perm {
		ret = append(ret, filtered[idx])
		if len(ret) >= maxResponses {
			break
		}
	}
	return ret, nil
}
