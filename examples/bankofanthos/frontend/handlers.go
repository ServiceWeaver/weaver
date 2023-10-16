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
	"crypto/rsa"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ServiceWeaver/weaver/examples/bankofanthos/contacts"
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/model"
	"github.com/ServiceWeaver/weaver/examples/bankofanthos/userservice"
	"github.com/golang-jwt/jwt"
)

const (
	tokenCookieName   = "token"
	consentCookieName = "consented"
)

var (
	isCymbalLogo = strings.ToLower(os.Getenv("CYMBAL_LOGO")) == "true"

	//go:embed templates/*
	templateFS embed.FS
	templates  = template.Must(template.New("").
			Funcs(template.FuncMap{
			"formatCurrency":       formatCurrency,
			"formatTimestampDay":   formatTimestampDay,
			"formatTimestampMonth": formatTimestampMonth,
		}).ParseFS(templateFS, "templates/*.html"))
)

func formatTimestampDay(t time.Time) string {
	return strconv.Itoa(t.Day())
}

func formatTimestampMonth(t time.Time) string {
	return t.Format(t.Month().String())
}

func formatCurrency(amount int64) string {
	amountStr := "$" + strconv.FormatFloat(float64(amount)/100.0, 'f', 2, 64)
	if amount < 0 {
		amountStr = "-" + amountStr
	}
	return amountStr
}

// decodeToken decodes a signed JWT using the provided publicKey and returns
// the parsed claims.
func decodeToken(token string, publicKey *rsa.PublicKey) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("Unexpected signing method")
		}
		return publicKey, nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// verifyToken validates the token using the provided public key.
func verifyToken(token string, publicKey *rsa.PublicKey) bool {
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("Unexpected signing method")
		}
		return publicKey, nil
	})
	if err != nil || !parsedToken.Valid {
		return false
	}
	return true
}

// rootHandler handles "/".
func (s *server) rootHandler(w http.ResponseWriter, r *http.Request) {
	token, err := r.Cookie(tokenCookieName)
	if err != nil || !verifyToken(token.Value, s.config.publicKey) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/home", http.StatusFound)
}

// homeHandler handles "/home" to render the home page. Redirects to "/login" if token is not valid.
func (s *server) homeHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.Logger(r.Context())
	token, err := r.Cookie(tokenCookieName)
	if err != nil || !verifyToken(token.Value, s.config.publicKey) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	// We can ignore the error below since the token has already been verified above.
	tokenData, _ := decodeToken(token.Value, s.config.publicKey)
	displayName := tokenData["name"].(string)
	username := tokenData["user"].(string)
	accountID := tokenData["acct"].(string)

	balance, err := s.balanceReader.Get().GetBalance(r.Context(), accountID)
	if err != nil {
		logger.Error("Couldn't fetch balance", err)
	}
	txnHistory, err := s.transactionHistory.Get().GetTransactions(r.Context(), accountID)
	if err != nil {
		logger.Error("Couldn't fetch transaction history", err)
	}
	contacts, err := s.contacts.Get().GetContacts(r.Context(), username)
	if err != nil {
		logger.Error("Couldn't fetch contacts", err)
	}
	labeledHistory := populateContactLabels(accountID, txnHistory, contacts)

	if err := templates.ExecuteTemplate(w, "index.html", map[string]interface{}{
		"ClusterName": s.config.clusterName,
		"PodName":     s.config.podName,
		"PodZone":     s.config.podZone,
		"CymbalLogo":  isCymbalLogo,
		"History":     labeledHistory,
		"Balance":     balance,
		"Name":        displayName,
		"AccountID":   accountID,
		"Contacts":    contacts,
		"Message":     r.URL.Query().Get("msg"),
		"BankName":    s.config.bankName,
	}); err != nil {
		logger.Error("couldn't generate home page", err)
	}
}

// LabeledTransaction adds an account label to Transaction.
type LabeledTransaction struct {
	model.Transaction
	AccountLabel string
}

// populateContactLabels returns a list of transactions labeled with respective account labels, if any.
func populateContactLabels(accountID string, transactions []model.Transaction, contacts []contacts.Contact) []LabeledTransaction {
	ret := []LabeledTransaction{}
	if accountID == "" || transactions == nil || contacts == nil {
		for _, t := range transactions {
			l := LabeledTransaction{Transaction: t}
			ret = append(ret, l)
		}
		return ret
	}

	contactMap := make(map[string]string)
	for _, c := range contacts {
		contactMap[c.AccountNum] = c.Label
	}
	for _, t := range transactions {
		var accountLabel string
		if t.ToAccountNum == accountID {
			accountLabel = contactMap[t.FromAccountNum]
		} else if t.FromAccountNum == accountID {
			accountLabel = contactMap[t.ToAccountNum]
		}
		l := LabeledTransaction{Transaction: t}
		l.AccountLabel = accountLabel
		ret = append(ret, l)
	}
	return ret
}

// paymentHandler handles "/payment" and submits payment request to ledgerwriter service.
func (s *server) paymentHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.Logger(r.Context())
	token, err := r.Cookie(tokenCookieName)
	if err != nil || !verifyToken(token.Value, s.config.publicKey) {
		msg := "Error submitting payment: user is not authenticated"
		logger.Error(msg, err)
		http.Error(w, msg, http.StatusUnauthorized)
		return
	}
	// We can ignore the error below since the token has already been verified above.
	tokenData, _ := decodeToken(token.Value, s.config.publicKey)
	authenticatedAccountID := tokenData["acct"].(string)
	recipient := r.FormValue("account_num")
	if recipient == "add" {
		recipient = r.FormValue("contact_account_num")
		label := r.FormValue("contact_label")
		// New contact. Add to contacts list.
		if label != "" {
			username := tokenData["user"].(string)
			contact := contacts.Contact{
				Username:   username,
				Label:      label,
				AccountNum: recipient,
				RoutingNum: s.config.localRoutingNum,
				IsExternal: false,
			}
			err := s.contacts.Get().AddContact(r.Context(), authenticatedAccountID, contact)
			if err != nil {
				http.Redirect(w, r, fmt.Sprintf("/home?msg=%s", err), http.StatusFound)
				return
			}
		}
	}

	userInput := r.FormValue("amount")
	paymentAmount, err := strconv.Atoi(userInput)
	if err != nil {
		http.Error(w, "Invalid amount", http.StatusBadRequest)
		return
	}
	paymentAmount *= 100

	txn := model.Transaction{
		FromAccountNum: authenticatedAccountID,
		FromRoutingNum: s.config.localRoutingNum,
		ToAccountNum:   recipient,
		ToRoutingNum:   s.config.localRoutingNum,
		Amount:         int64(paymentAmount),
		Timestamp:      time.Now(),
	}
	err = s.ledgerWriter.Get().AddTransaction(r.Context(), r.FormValue("uuid"), authenticatedAccountID, txn)
	// Add a short delay to allow the transaction to propogate to balancereader and
	// transactionhistory caches.
	time.Sleep(250 * time.Millisecond)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logger.Info("Payment initiated successfully")
	http.Redirect(w, r, "/home?msg=Payment successful", http.StatusSeeOther)
}

// depositHandler handles "/deposit" and submits deposit requests to ledgerwriter service.
func (s *server) depositHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.Logger(r.Context())
	token, err := r.Cookie(tokenCookieName)
	if err != nil || !verifyToken(token.Value, s.config.publicKey) {
		msg := "Error submitting deposit: user is not authenticated"
		logger.Error(msg, err)
		http.Error(w, msg, http.StatusUnauthorized)
		return
	}
	// We can ignore the error below since the token has already been verified above.
	tokenData, _ := decodeToken(token.Value, s.config.publicKey)
	authenticatedAccountID := tokenData["acct"].(string)
	var externalAccountNum string
	var externalRoutingNum string
	if r.FormValue("account") == "add" {
		externalAccountNum = r.FormValue("external_account_num")
		externalRoutingNum = r.FormValue("external_routing_num")
		if externalRoutingNum == s.config.localRoutingNum {
			msg := "Error submitting deposit: invalid routing number"
			http.Redirect(w, r, fmt.Sprintf("/home?msg=%s", msg), http.StatusFound)
			return
		}
		externalLabel := r.FormValue("external_label")
		if externalLabel != "" {
			// New contact. Add to contacts list.
			username := tokenData["user"].(string)
			contact := contacts.Contact{
				Username:   username,
				Label:      externalLabel,
				AccountNum: externalAccountNum,
				RoutingNum: externalRoutingNum,
				IsExternal: true,
			}
			err := s.contacts.Get().AddContact(r.Context(), authenticatedAccountID, contact)
			if err != nil {
				msg := fmt.Sprintf("Deposit failed: %v", err)
				http.Redirect(w, r, fmt.Sprintf("/home?msg=%s", msg), http.StatusFound)
				return
			}
		}
	} else {
		accountBytes := []byte(r.FormValue("account"))
		accountDetails := make(map[string]interface{})
		if err := json.Unmarshal(accountBytes, &accountDetails); err != nil {
			msg := "Deposit failed: request has malformed 'account' param"
			http.Redirect(w, r, fmt.Sprintf("/home?msg=%s", msg), http.StatusFound)
			return
		}
		var ok bool
		externalAccountNum, ok = accountDetails["account_num"].(string)
		if !ok {
			msg := "Deposit failed: request doesn't specify account_num"
			http.Redirect(w, r, fmt.Sprintf("/home?msg=%s", msg), http.StatusFound)
			return
		}
		externalRoutingNum, ok = accountDetails["routing_num"].(string)
		if !ok {
			msg := "Deposit failed: request doesn't specify routing_num"
			http.Redirect(w, r, fmt.Sprintf("/home?msg=%s", msg), http.StatusFound)
			return
		}
	}

	userInput := r.FormValue("amount")
	paymentAmount, err := strconv.Atoi(userInput)
	if err != nil {
		http.Error(w, "Invalid amount", http.StatusBadRequest)
		return
	}
	paymentAmount *= 100

	txn := model.Transaction{
		FromAccountNum: externalAccountNum,
		FromRoutingNum: externalRoutingNum,
		ToAccountNum:   authenticatedAccountID,
		ToRoutingNum:   s.config.localRoutingNum,
		Amount:         int64(paymentAmount),
		Timestamp:      time.Now(),
	}
	err = s.ledgerWriter.Get().AddTransaction(r.Context(), r.FormValue("uuid"), authenticatedAccountID, txn)
	// Add a short delay to allow the transaction to propogate to balancereader and
	// transactionhistory caches.
	time.Sleep(250 * time.Millisecond)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logger.Info("Deposit initiated successfully")
	http.Redirect(w, r, "/home?msg=Deposit successful", http.StatusSeeOther)
}

// loginHandler handles "/login".
func (s *server) loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.loginGetHandler(w, r)
	} else if r.Method == http.MethodPost {
		s.loginPostHandler(w, r)
	}
}

// loginGetHandler renders the login page. Redirects to "/home" if user already has a valid token.
// If this is an oauth flow, then redirect to a consent form.
func (s *server) loginGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.Logger(r.Context())
	redirect := func(uri string, values map[string]string) {
		url, _ := url.Parse(uri)
		query := url.Query()
		for k, v := range values {
			query.Set(k, v)
		}
		url.RawQuery = query.Encode()
		http.Redirect(w, r, url.String(), http.StatusFound)
	}

	responseType := r.URL.Query().Get("response_type")
	clientID := r.URL.Query().Get("client_id")
	appName := r.URL.Query().Get("app_name")
	redirectURI := r.URL.Query().Get("redirect_uri")
	state := r.URL.Query().Get("state")
	token, tokenErr := r.Cookie(tokenCookieName)
	registeredOauthClientID := os.Getenv("REGISTERED_OAUTH_CLIENT_ID")
	allowedOauthRedirectURI := os.Getenv("ALLOWED_OAUTH_REDIRECT_URI")
	if registeredOauthClientID != "" && allowedOauthRedirectURI != "" && responseType == "code" {
		logger.Debug("Login with response_type=code")
		if clientID != registeredOauthClientID {
			redirect("/login", map[string]string{"msg": "Error: invalid client_id"})
			return
		}
		if redirectURI != allowedOauthRedirectURI {
			redirect("/login", map[string]string{"msg": "Error: invalid redirect_id"})
			return
		}

		if verifyToken(token.Value, s.config.publicKey) {
			logger.Debug("User already authenticated. Redirecting to /consent")
			redirect("/consent", map[string]string{
				"state":        state,
				"redirect_uri": redirectURI,
				"app_name":     appName,
			})
			return
		}
	} else if tokenErr == nil && verifyToken(token.Value, s.config.publicKey) {
		logger.Debug("User already authenticated. Redirecting to /home")
		http.Redirect(w, r, "/home", http.StatusFound)
		return
	}

	if err := templates.ExecuteTemplate(w, "login.html", map[string]interface{}{
		"CymbalLogo":      isCymbalLogo,
		"ClusterName":     s.config.clusterName,
		"PodName":         s.config.podName,
		"PodZone":         s.config.podZone,
		"Message":         r.URL.Query().Get("msg"),
		"DefaultUser":     os.Getenv("DEFAULT_USERNAME"),
		"DefaultPassword": os.Getenv("DEFAULT_PASSWORD"),
		"BankName":        s.config.bankName,
		"ResponseType":    responseType,
		"State":           state,
		"RedirectURI":     redirectURI,
		"AppName":         appName,
	}); err != nil {
		logger.Error("couldn't generate login page", err)
	}
}

// loginPostHandler handles POST requests to "/login" and contacts the userservice
// to return a signed JWT for authentication and for authorization of subsequent
// requests by the user.
func (s *server) loginPostHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.Logger(r.Context())
	err := s.loginPostHelper(w, r)
	if err != nil {
		logger.Error("/login POST failed", err, "user", r.FormValue("username"))
		http.Redirect(w, r, "/login?msg=Login+Failed", http.StatusFound)
	}
}

func (s *server) loginPostHelper(w http.ResponseWriter, r *http.Request) error {
	logger := s.Logger(r.Context())
	loginReq := userservice.LoginRequest{
		Username: r.FormValue("username"),
		Password: r.FormValue("password"),
	}
	token, err := s.userService.Get().Login(r.Context(), loginReq)
	if err != nil {
		return err
	}
	claims, err := decodeToken(token, s.config.publicKey)
	if err != nil {
		return fmt.Errorf("unable to decode jwt token: %w", err)
	}
	maxAge := int(claims["exp"].(float64) - claims["iat"].(float64))
	cookie := &http.Cookie{
		Name:   tokenCookieName,
		Value:  token,
		MaxAge: maxAge,
	}
	http.SetCookie(w, cookie)

	urlValues := r.URL.Query()
	if urlValues.Has("response_type") && urlValues.Has("state") &&
		urlValues.Has("redirect_uri") && urlValues.Get("response_type") == "code" {
		url, _ := url.Parse("/consent")
		query := url.Query()
		query.Set("state", urlValues.Get("state"))
		query.Set("redirect_uri", urlValues.Get("redirect_uri"))
		query.Set("app_name", urlValues.Get("app_name"))
		url.RawQuery = query.Encode()
		logger.Debug("Login successful, redirecting to /consent")
		http.Redirect(w, r, url.String(), http.StatusFound)
	} else {
		logger.Debug("Login successful, redirecting to /home")
		http.Redirect(w, r, "/home", http.StatusFound)
	}
	return nil
}

func (s *server) authCallbackHelper(state string, redirectURI string, token string, w http.ResponseWriter, r *http.Request, setConsentCookie bool) {
	if redirectURI != "" {
		// NOTE: We disallow arbitrary redirects for security reasons. Because
		// Bank of Anthos is a demo app, this is okay. For a real app, you
		// would need a more sophisticated mechanism to allow redirects.
		http.Error(w, fmt.Sprintf("disallowed redirect URI: %q", redirectURI), http.StatusForbidden)
		return
	}

	client := &http.Client{Timeout: s.config.backendTimeout}
	formData := url.Values{
		"state":    {state},
		"id_token": {token},
	}
	if setConsentCookie {
		cookie := &http.Cookie{
			Name:  consentCookieName,
			Value: "true",
		}
		http.SetCookie(w, cookie)
	}
	req, err := http.NewRequest("POST", redirectURI, strings.NewReader(formData.Encode()))
	if err != nil {
		http.Redirect(w, r, redirectURI+"#error=server_error", http.StatusFound)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)

	if err != nil {
		http.Redirect(w, r, redirectURI+"#error=server_error", http.StatusFound)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusFound {
		location := resp.Header.Get("Location")
		http.Redirect(w, r, location, http.StatusFound)
		return
	}
	http.Redirect(w, r, redirectURI+"#error=server_error", http.StatusFound)
}

// consentHandler handles GET and POST requests to "/consent".
func (s *server) consentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.consentGetHandler(w, r)
	} else if r.Method == http.MethodPost {
		s.consentPostHandler(w, r)
	}
}

// consentGetHandler renders consent page.  Retrieves auth code if the user
// already logged in and consented.
func (s *server) consentGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.Logger(r.Context())
	values := r.URL.Query()
	redirectURI := values.Get("redirect_uri")
	state := values.Get("state")
	appName := values.Get("app_name")

	redirectToLogin := func() {
		url, _ := url.Parse("/login")
		query := url.Query()
		query.Set("response_type", "code")
		query.Set("state", state)
		query.Set("redirect_uri", redirectURI)
		query.Set("app_name", appName)
		url.RawQuery = query.Encode()
		http.Redirect(w, r, url.String(), http.StatusFound)
	}

	token, err := r.Cookie(tokenCookieName)
	if err != nil {
		redirectToLogin()
	}
	consented := values.Get(consentCookieName)
	if verifyToken(token.Value, s.config.publicKey) {
		if consented == "true" {
			logger.Debug("User consent already granted")
			s.authCallbackHelper(state, redirectURI, token.Value, w, r, false)
			return
		}
		if err := templates.ExecuteTemplate(w, "consent.html", map[string]interface{}{
			"CymbalLogo":  isCymbalLogo,
			"ClusterName": s.config.clusterName,
			"PodName":     s.config.podName,
			"PodZone":     s.config.podZone,
			"BankName":    s.config.bankName,
			"State":       state,
			"RedirectURI": redirectURI,
			"AppName":     appName,
		}); err != nil {
			logger.Error("couldn't generate consent.html", err)
		}
		return
	}
	redirectToLogin()
}

func (s *server) consentPostHandler(w http.ResponseWriter, r *http.Request) {
	// Check consent, write cookie if yes, and redirect accordingly.
	consent := r.URL.Query().Get("consent")
	state := r.URL.Query().Get("state")
	redirectURI := r.URL.Query().Get("redirect_uri")
	token, err := r.Cookie(tokenCookieName)
	if err != nil {
		http.Redirect(w, r, redirectURI+"#error=invalid_token", http.StatusFound)
	}
	if consent == "true" {
		s.authCallbackHelper(state, redirectURI, token.Value, w, r, true)
	} else {
		http.Redirect(w, r, redirectURI+"#error=access_denied", http.StatusFound)
	}
}

// signupHandler handles GET and POST requests to "/signup".
func (s *server) signupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.signupGetHandler(w, r)
	} else if r.Method == http.MethodPost {
		s.signupPostHandler(w, r)
	}
}

// signupGetHandler handles renders the signup page.
func (s *server) signupGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.Logger(r.Context())
	token, err := r.Cookie(tokenCookieName)
	if err == nil && verifyToken(token.Value, s.config.publicKey) {
		http.Redirect(w, r, "/home", http.StatusFound)
		return
	}
	if err := templates.ExecuteTemplate(w, "signup.html", map[string]interface{}{
		"CymbalLogo":  isCymbalLogo,
		"ClusterName": s.config.clusterName,
		"PodName":     s.config.podName,
		"PodZone":     s.config.podZone,
		"BankName":    s.config.bankName,
	}); err != nil {
		logger.Error("couldn't generate consent.html", err)
	}
}

// signupPostHandler handles POST requests to "/signup" by contacting userservice
// for creating a new user.
func (s *server) signupPostHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.Logger(r.Context())
	logger.Debug("Creating new user")

	creq := userservice.CreateUserRequest{
		Username:       r.FormValue("username"),
		Password:       r.FormValue("password"),
		PasswordRepeat: r.FormValue("password-repeat"),
		FirstName:      r.FormValue("firstname"),
		LastName:       r.FormValue("lastname"),
		Birthday:       r.FormValue("birthday"),
		Timezone:       r.FormValue("timezone"),
		Address:        r.FormValue("address"),
		State:          r.FormValue("state"),
		Zip:            r.FormValue("zip"),
		Ssn:            r.FormValue("ssn"),
	}
	err := s.userService.Get().CreateUser(r.Context(), creq)
	if err != nil {
		logger.Debug("Error creating new user", "err", err)
		url := fmt.Sprintf("/login?msg=Account creation failed: %s", err)
		http.Redirect(w, r, url, http.StatusSeeOther)
		return
	}

	logger.Info("New user created.")
	if err := s.loginPostHelper(w, r); err != nil {
		http.Redirect(w, r, "/login?msg=Login+Failed", http.StatusFound)
	}
}

// logoutHandler handles "/logout" by deleting appropriate cookies.
func (s *server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	// Delete the token and consent cookies.
	http.SetCookie(w, &http.Cookie{
		Name:   tokenCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:   consentCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
