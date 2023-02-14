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

package currencyservice

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/types/money"
	"golang.org/x/exp/maps"
)

var (
	//go:embed data/currency_conversion.json
	currencyData []byte
)

type T interface {
	GetSupportedCurrencies(ctx context.Context) ([]string, error)
	Convert(ctx context.Context, from money.T, toCode string) (money.T, error)
}

type impl struct {
	weaver.Implements[T]
	conversionMap map[string]float64
}

func (s *impl) Init(context.Context) error {
	m, err := createConversionMap()
	s.conversionMap = m
	return err
}

// GetSupportedCurrencies returns the list of supported currencies.
func (s *impl) GetSupportedCurrencies(ctx context.Context) ([]string, error) {
	s.Logger().Info("Getting supported currencies...")
	return maps.Keys(s.conversionMap), nil
}

// Convert converts between currencies.
func (s *impl) Convert(ctx context.Context, from money.T, toCode string) (money.T, error) {
	unsupportedErr := func(code string) (money.T, error) {
		return money.T{}, fmt.Errorf("unsupported currency code %q", from.CurrencyCode)
	}

	// Convert: from --> EUR
	fromRate, ok := s.conversionMap[from.CurrencyCode]
	if !ok {
		return unsupportedErr(from.CurrencyCode)
	}
	euros := carry(float64(from.Units)/fromRate, float64(from.Nanos)/fromRate)

	// Convert: EUR -> toCode
	toRate, ok := s.conversionMap[toCode]
	if !ok {
		return unsupportedErr(toCode)
	}
	to := carry(float64(euros.Units)*toRate, float64(euros.Nanos)*toRate)
	to.CurrencyCode = toCode
	return to, nil
}

// carry is a helper function that handles decimal/fractional carrying.
func carry(units float64, nanos float64) money.T {
	const fractionSize = 1000000000 // 1B
	nanos += math.Mod(units, 1.0) * fractionSize
	units = math.Floor(units) + math.Floor(nanos/fractionSize)
	nanos = math.Mod(nanos, fractionSize)
	return money.T{
		Units: int64(units),
		Nanos: int32(nanos),
	}
}

func createConversionMap() (map[string]float64, error) {
	m := map[string]string{}
	if err := json.Unmarshal(currencyData, &m); err != nil {
		return nil, err
	}
	conv := make(map[string]float64, len(m))
	for k, v := range m {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, err
		}
		conv[k] = f
	}
	return conv, nil
}
