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

package frontend

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ServiceWeaver/weaver/examples/onlineboutique/adservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/cartservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/checkoutservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/paymentservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/productcatalogservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/shippingservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/types/money"
	"golang.org/x/exp/slog"
)

const (
	avoidNoopCurrencyConversionRPC = false
)

var (
	isCymbalBrand = strings.ToLower(os.Getenv("CYMBAL_BRANDING")) == "true"

	//go:embed templates/*
	templateFS embed.FS
	templates  = template.Must(template.New("").
			Funcs(template.FuncMap{
			"renderMoney":        renderMoney,
			"renderCurrencyLogo": renderCurrencyLogo,
		}).ParseFS(templateFS, "templates/*.html"))

	allowlistedCurrencies = map[string]bool{
		"USD": true,
		"EUR": true,
		"CAD": true,
		"JPY": true,
		"GBP": true,
		"TRY": true,
	}

	defaultCurrency = "USD"
)

func (fe *Server) homeHandler(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(ctxKeyLogger{}).(*slog.Logger)
	logger.Info("home", "currency", currentCurrency(r))
	currencies, err := fe.getCurrencies(r.Context())
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("could not retrieve currencies: %w", err), http.StatusInternalServerError)
		return
	}
	products, err := fe.catalogService.ListProducts(r.Context())
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("could not retrieve products: %w", err), http.StatusInternalServerError)
		return
	}
	cart, err := fe.cartService.GetCart(r.Context(), sessionID(r))
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("could not retrieve cart: %w", err), http.StatusInternalServerError)
		return
	}

	type productView struct {
		Item  productcatalogservice.Product
		Price money.T
	}
	ps := make([]productView, len(products))
	for i, p := range products {
		price, err := fe.currencyService.Convert(r.Context(), p.PriceUSD, currentCurrency(r))
		if err != nil {
			fe.renderHTTPError(r, w, fmt.Errorf("failed to do currency conversion for product %s: %w", p.ID, err), http.StatusInternalServerError)
			return
		}
		ps[i] = productView{p, price}
	}

	if err := templates.ExecuteTemplate(w, "home", map[string]interface{}{
		"session_id":      sessionID(r),
		"request_id":      r.Context().Value(ctxKeyRequestID{}),
		"hostname":        fe.hostname,
		"user_currency":   currentCurrency(r),
		"show_currency":   true,
		"currencies":      currencies,
		"products":        ps,
		"cart_size":       cartSize(cart),
		"banner_color":    os.Getenv("BANNER_COLOR"),
		"ad":              fe.chooseAd(r.Context(), []string{}, logger),
		"platform_css":    fe.platform.css,
		"platform_name":   fe.platform.provider,
		"is_cymbal_brand": isCymbalBrand,
	}); err != nil {
		logger.Error("generate home page", "err", err)
	}
}

func (fe *Server) productHandler(w http.ResponseWriter, r *http.Request) {
	_, id := filepath.Split(r.URL.Path)
	logger := r.Context().Value(ctxKeyLogger{}).(*slog.Logger)
	if id == "" {
		fe.renderHTTPError(r, w, errors.New("product id not specified"), http.StatusBadRequest)
		return
	}
	logger.Debug("serving product page", "id", id, "currency", currentCurrency(r))

	p, err := fe.catalogService.GetProduct(r.Context(), id)
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("could not retrieve product: %w", err), http.StatusInternalServerError)
		return
	}
	currencies, err := fe.getCurrencies(r.Context())
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("could not retrieve currencies: %w", err), http.StatusInternalServerError)
		return
	}

	cart, err := fe.cartService.GetCart(r.Context(), sessionID(r))
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("could not retrieve cart: %w", err), http.StatusInternalServerError)
		return
	}

	price, err := fe.convertCurrency(r.Context(), p.PriceUSD, currentCurrency(r))
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("failed to convert currency: %w", err), http.StatusInternalServerError)
		return
	}

	recommendations, err := fe.getRecommendations(r.Context(), sessionID(r), []string{id})
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("failed to get product recommendations: %w", err), http.StatusInternalServerError)
		return
	}

	product := struct {
		Item  productcatalogservice.Product
		Price money.T
	}{p, price}

	if err := templates.ExecuteTemplate(w, "product", map[string]interface{}{
		"session_id":      sessionID(r),
		"request_id":      r.Context().Value(ctxKeyRequestID{}),
		"hostname":        fe.hostname,
		"ad":              fe.chooseAd(r.Context(), p.Categories, logger),
		"user_currency":   currentCurrency(r),
		"show_currency":   true,
		"currencies":      currencies,
		"product":         product,
		"recommendations": recommendations,
		"cart_size":       cartSize(cart),
		"platform_css":    fe.platform.css,
		"platform_name":   fe.platform.provider,
		"is_cymbal_brand": isCymbalBrand,
	}); err != nil {
		logger.Error("generate product page", "err", err)
	}
}

func (fe *Server) cartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		fe.viewCartHandler(w, r)
		return
	}
	if r.Method == http.MethodPost {
		fe.addToCartHandler(w, r)
		return
	}
	msg := fmt.Sprintf("method %q not allowed", r.Method)
	http.Error(w, msg, http.StatusMethodNotAllowed)
}

func (fe *Server) addToCartHandler(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(ctxKeyLogger{}).(*slog.Logger)
	quantity, _ := strconv.ParseUint(r.FormValue("quantity"), 10, 32)
	productID := r.FormValue("product_id")
	if productID == "" || quantity == 0 {
		fe.renderHTTPError(r, w, errors.New("invalid form input"), http.StatusBadRequest)
		return
	}
	logger.Debug("adding to cart", "product", productID, "quantity", quantity)

	p, err := fe.catalogService.GetProduct(r.Context(), productID)
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("could not retrieve product: %w", err), http.StatusInternalServerError)
		return
	}

	if err := fe.cartService.AddItem(r.Context(), sessionID(r), cartservice.CartItem{
		ProductID: p.ID,
		Quantity:  int32(quantity),
	}); err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("failed to add to cart: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("location", "/cart")
	w.WriteHeader(http.StatusFound)
}

func (fe *Server) emptyCartHandler(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(ctxKeyLogger{}).(*slog.Logger)
	logger.Debug("emptying cart")

	if err := fe.cartService.EmptyCart(r.Context(), sessionID(r)); err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("failed to empty cart: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("location", "/")
	w.WriteHeader(http.StatusFound)
}

func (fe *Server) viewCartHandler(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(ctxKeyLogger{}).(*slog.Logger)
	logger.Debug("view user cart")
	currencies, err := fe.getCurrencies(r.Context())
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("could not retrieve currencies: %w", err), http.StatusInternalServerError)
		return
	}
	cart, err := fe.cartService.GetCart(r.Context(), sessionID(r))
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("could not retrieve cart: %w", err), http.StatusInternalServerError)
		return
	}

	recommendations, err := fe.getRecommendations(r.Context(), sessionID(r), cartIDs(cart))
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("failed to get product recommendations: %w", err), http.StatusInternalServerError)
		return
	}

	shippingCost, err := fe.getShippingQuote(r.Context(), cart, currentCurrency(r))
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("failed to get shipping quote: %w", err), http.StatusInternalServerError)
		return
	}

	type cartItemView struct {
		Item     productcatalogservice.Product
		Quantity int32
		Price    *money.T
	}
	items := make([]cartItemView, len(cart))
	totalPrice := money.T{CurrencyCode: currentCurrency(r)}
	for i, item := range cart {
		p, err := fe.catalogService.GetProduct(r.Context(), item.ProductID)
		if err != nil {
			fe.renderHTTPError(r, w, fmt.Errorf("could not retrieve product #%s: %w", item.ProductID, err), http.StatusInternalServerError)
			return
		}
		price, err := fe.convertCurrency(r.Context(), p.PriceUSD, currentCurrency(r))
		if err != nil {
			fe.renderHTTPError(r, w, fmt.Errorf("could not convert currency for product #%s: %w", item.ProductID, err), http.StatusInternalServerError)
			return
		}

		multPrice := money.MultiplySlow(price, uint32(item.Quantity))
		items[i] = cartItemView{
			Item:     p,
			Quantity: item.Quantity,
			Price:    &multPrice}
		totalPrice = money.Must(money.Sum(totalPrice, multPrice))
	}
	totalPrice = money.Must(money.Sum(totalPrice, shippingCost))
	year := time.Now().Year()

	if err := templates.ExecuteTemplate(w, "cart", map[string]interface{}{
		"session_id":       sessionID(r),
		"request_id":       r.Context().Value(ctxKeyRequestID{}),
		"hostname":         fe.hostname,
		"user_currency":    currentCurrency(r),
		"currencies":       currencies,
		"recommendations":  recommendations,
		"cart_size":        cartSize(cart),
		"shipping_cost":    shippingCost,
		"show_currency":    true,
		"total_cost":       totalPrice,
		"items":            items,
		"expiration_years": []int{year, year + 1, year + 2, year + 3, year + 4},
		"platform_css":     fe.platform.css,
		"platform_name":    fe.platform.provider,
		"is_cymbal_brand":  isCymbalBrand,
	}); err != nil {
		logger.Error("generate cart page", "err", err)
	}
}

func (fe *Server) placeOrderHandler(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(ctxKeyLogger{}).(*slog.Logger)
	logger.Debug("placing order")

	var (
		email         = r.FormValue("email")
		streetAddress = r.FormValue("street_address")
		zipCode, _    = strconv.ParseInt(r.FormValue("zip_code"), 10, 32)
		city          = r.FormValue("city")
		state         = r.FormValue("state")
		country       = r.FormValue("country")
		ccNumber      = r.FormValue("credit_card_number")
		ccMonth, _    = strconv.ParseInt(r.FormValue("credit_card_expiration_month"), 10, 32)
		ccYear, _     = strconv.ParseInt(r.FormValue("credit_card_expiration_year"), 10, 32)
		ccCVV, _      = strconv.ParseInt(r.FormValue("credit_card_cvv"), 10, 32)
	)

	order, err := fe.checkoutService.PlaceOrder(r.Context(), checkoutservice.PlaceOrderRequest{
		Email: email,
		CreditCard: paymentservice.CreditCardInfo{
			Number:          ccNumber,
			ExpirationMonth: time.Month(ccMonth),
			ExpirationYear:  int(ccYear),
			CVV:             int32(ccCVV)},
		UserID:       sessionID(r),
		UserCurrency: currentCurrency(r),
		Address: shippingservice.Address{
			StreetAddress: streetAddress,
			City:          city,
			State:         state,
			ZipCode:       int32(zipCode),
			Country:       country},
	})
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("failed to complete the order: %w", err), http.StatusInternalServerError)
		return
	}
	logger.Info("order placed", "id", order.OrderID)

	totalPaid := order.ShippingCost
	for _, item := range order.Items {
		multPrice := money.MultiplySlow(item.Cost, uint32(item.Item.Quantity))
		totalPaid = money.Must(money.Sum(totalPaid, multPrice))
	}

	currencies, err := fe.getCurrencies(r.Context())
	if err != nil {
		fe.renderHTTPError(r, w, fmt.Errorf("could not retrieve currencies: %w", err), http.StatusInternalServerError)
		return
	}

	recommendations, _ := fe.getRecommendations(r.Context(), sessionID(r), nil /*productIDs*/)

	if err := templates.ExecuteTemplate(w, "order", map[string]interface{}{
		"session_id":      sessionID(r),
		"request_id":      r.Context().Value(ctxKeyRequestID{}),
		"hostname":        fe.hostname,
		"user_currency":   currentCurrency(r),
		"show_currency":   false,
		"currencies":      currencies,
		"order":           order,
		"total_paid":      &totalPaid,
		"recommendations": recommendations,
		"platform_css":    fe.platform.css,
		"platform_name":   fe.platform.provider,
		"is_cymbal_brand": isCymbalBrand,
	}); err != nil {
		logger.Error("generate order page", "err", err)
	}
}

func (fe *Server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(ctxKeyLogger{}).(*slog.Logger)
	logger.Debug("logging out")
	for _, c := range r.Cookies() {
		c.Expires = time.Now().Add(-time.Hour * 24 * 365)
		c.MaxAge = -1
		http.SetCookie(w, c)
	}
	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusFound)
}

func (fe *Server) setCurrencyHandler(w http.ResponseWriter, r *http.Request) {
	logger := r.Context().Value(ctxKeyLogger{}).(*slog.Logger)
	cur := r.FormValue("currency_code")
	logger.Debug("setting currency", "curr.new", cur, "curr.old", currentCurrency(r))

	if cur != "" {
		http.SetCookie(w, &http.Cookie{
			Name:   cookieCurrency,
			Value:  cur,
			MaxAge: cookieMaxAge,
		})
	}
	referer := r.Header.Get("referer")
	if referer == "" {
		referer = "/"
	}
	w.Header().Set("Location", referer)
	w.WriteHeader(http.StatusFound)
}

// chooseAd queries for advertisements available and randomly chooses one, if
// available. It ignores the error retrieving the ad since it is not critical.
func (fe *Server) chooseAd(ctx context.Context, ctxKeys []string, logger *slog.Logger) *adservice.Ad {
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*100)
	defer cancel()
	ads, err := fe.adService.GetAds(ctx, ctxKeys)
	if err != nil {
		logger.Error("failed to retrieve ads", "err", err)
		return nil
	}
	return &ads[rand.Intn(len(ads))]
}

func (fe *Server) getCurrencies(ctx context.Context) ([]string, error) {
	codes, err := fe.currencyService.GetSupportedCurrencies(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range codes {
		if _, ok := allowlistedCurrencies[c]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func (fe *Server) convertCurrency(ctx context.Context, money money.T, currency string) (money.T, error) {
	if avoidNoopCurrencyConversionRPC && money.CurrencyCode == currency {
		return money, nil
	}
	return fe.currencyService.Convert(ctx, money, currency)
}

func (fe *Server) getShippingQuote(ctx context.Context, items []cartservice.CartItem, currency string) (money.T, error) {
	quote, err := fe.shippingService.GetQuote(ctx, shippingservice.Address{}, items)
	if err != nil {
		return money.T{}, err
	}
	return fe.convertCurrency(ctx, quote, currency)
}

func (fe *Server) getRecommendations(ctx context.Context, userID string, productIDs []string) ([]productcatalogservice.Product, error) {
	recommendationIDs, err := fe.recommendationService.ListRecommendations(ctx, userID, productIDs)
	if err != nil {
		return nil, err
	}
	out := make([]productcatalogservice.Product, len(recommendationIDs))
	for i, id := range recommendationIDs {
		p, err := fe.catalogService.GetProduct(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get recommended product info (#%s): %w", id, err)
		}
		out[i] = p
	}
	if len(out) > 4 {
		out = out[:4] // take only first four to fit the UI
	}
	return out, err
}

func (fe *Server) renderHTTPError(r *http.Request, w http.ResponseWriter, err error, code int) {
	logger := r.Context().Value(ctxKeyLogger{}).(*slog.Logger)
	logger.Error("request error", "err", err)
	errMsg := fmt.Sprintf("%+v", err)

	w.WriteHeader(code)

	if templateErr := templates.ExecuteTemplate(w, "error", map[string]interface{}{
		"session_id":  sessionID(r),
		"request_id":  r.Context().Value(ctxKeyRequestID{}),
		"hostname":    fe.hostname,
		"error":       errMsg,
		"status_code": code,
		"status":      http.StatusText(code),
	}); templateErr != nil {
		logger.Error("generate error page", "err", templateErr)
	}
}

func currentCurrency(r *http.Request) string {
	c, _ := r.Cookie(cookieCurrency)
	if c != nil {
		return c.Value
	}
	return defaultCurrency
}

func sessionID(r *http.Request) string {
	v := r.Context().Value(ctxKeySessionID{})
	if v != nil {
		return v.(string)
	}
	return ""
}

func cartIDs(c []cartservice.CartItem) []string {
	out := make([]string, len(c))
	for i, v := range c {
		out[i] = v.ProductID
	}
	return out
}

// get total # of items in cart
func cartSize(c []cartservice.CartItem) int {
	cartSize := 0
	for _, item := range c {
		cartSize += int(item.Quantity)
	}
	return cartSize
}

func renderMoney(m money.T) string {
	currencyLogo := renderCurrencyLogo(m.CurrencyCode)
	return fmt.Sprintf("%s%d.%02d", currencyLogo, m.Units, m.Nanos/10000000)
}

func renderCurrencyLogo(currencyCode string) string {
	logos := map[string]string{
		"USD": "$",
		"CAD": "$",
		"JPY": "¥",
		"EUR": "€",
		"TRY": "₺",
		"GBP": "£",
	}

	logo := "$" //default
	if val, ok := logos[currencyCode]; ok {
		logo = val
	}
	return logo
}

func stringinSlice(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
