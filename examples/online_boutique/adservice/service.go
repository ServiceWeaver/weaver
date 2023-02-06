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

package adservice

import (
	"context"
	"math/rand"
	"strings"

	"golang.org/x/exp/maps"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	weaver "github.com/ServiceWeaver/weaver"
)

const (
	maxAdsToServe = 2
)

// Ad represents an advertisement.
type Ad struct {
	weaver.AutoMarshal
	RedirectURL string // URL to redirect to when an ad is clicked.
	Text        string // Short advertisement text to display.
}

type T interface {
	GetAds(ctx context.Context, keywords []string) ([]Ad, error)
}

type impl struct {
	weaver.Implements[T]
	ads map[string]Ad
}

func (s *impl) Init(context.Context) error {
	s.Logger().Info("Ad Service started")
	s.ads = createAdsMap()
	return nil
}

// GetAds returns a list of ads that best match the given context keywords.
func (s *impl) GetAds(ctx context.Context, keywords []string) ([]Ad, error) {
	s.Logger().Info("received ad request", "keywords", keywords)
	span := trace.SpanFromContext(ctx)
	var allAds []Ad
	if len(keywords) > 0 {
		span.AddEvent("Constructing Ads using context", trace.WithAttributes(
			attribute.String("Context Keys", strings.Join(keywords, ",")),
			attribute.Int("Context Keys length", len(keywords)),
		))
		for _, kw := range keywords {
			allAds = append(allAds, s.getAdsByCategory(kw)...)
		}
		if allAds == nil {
			// Serve random ads.
			span.AddEvent("No Ads found based on context. Constructing random Ads.")
			allAds = s.getRandomAds()
		}
	} else {
		span.AddEvent("No Context provided. Constructing random Ads.")
		allAds = s.getRandomAds()
	}
	return allAds, nil
}

func (s *impl) getAdsByCategory(category string) []Ad {
	return []Ad{s.ads[category]}
}

func (s *impl) getRandomAds() []Ad {
	ads := make([]Ad, maxAdsToServe)
	vals := maps.Values(s.ads)
	for i := 0; i < maxAdsToServe; i++ {
		ads[i] = vals[rand.Intn(len(vals))]
	}
	return ads
}

func createAdsMap() map[string]Ad {
	return map[string]Ad{
		"hair": {
			RedirectURL: "/product/2ZYFJ3GM2N",
			Text:        "Hairdryer for sale. 50% off.",
		},
		"clothing": {
			RedirectURL: "/product/66VCHSJNUP",
			Text:        "Tank top for sale. 20% off.",
		},
		"accessories": {
			RedirectURL: "/product/1YMWWN1N4O",
			Text:        "Watch for sale. Buy one, get second kit for free",
		},
		"footwear": {
			RedirectURL: "/product/L9ECAV7KIM",
			Text:        "Loafers for sale. Buy one, get second one for free",
		},
		"decor": {
			RedirectURL: "/product/0PUK6V6EV0",
			Text:        "Candle holder for sale. 30% off.",
		},
		"kitchen": {
			RedirectURL: "/product/9SIQT8TOJO",
			Text:        "Bamboo glass jar for sale. 10% off.",
		},
	}
}
