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

package contacts

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/ServiceWeaver/weaver"
)

type T interface {
	// GetContacts returns a list of contacts of a user.
	GetContacts(ctx context.Context, username string) ([]Contact, error)
	// AddContact adds a new contact for a user.
	AddContact(ctx context.Context, authenticatedAccountID string, contact Contact) error
}

type config struct {
	AccountDBURI    string `toml:"account_db_uri"`
	PublicKeyPath   string `toml:"public_key_path"`
	LocalRoutingNum string `toml:"local_routing_num"`
}

type impl struct {
	weaver.Implements[T]
	weaver.WithConfig[config]
	db        *contactDB
	publicKey string
}

func (i *impl) Init(context.Context) error {
	publicKeyBytes, err := os.ReadFile(i.Config().PublicKeyPath)
	if err != nil {
		return err
	}
	i.publicKey = string(publicKeyBytes)

	i.db, err = newContactDB(i.Config().AccountDBURI)
	return err
}

func (i *impl) GetContacts(ctx context.Context, username string) ([]Contact, error) {
	return i.db.getContacts(username)
}

func (i *impl) AddContact(ctx context.Context, authenticatedAccountID string, contact Contact) error {
	err := i.validateNewContact(contact)
	if err != nil {
		return fmt.Errorf("error adding contact: %w", err)
	}

	err = i.checkContactAllowed(contact.Username, authenticatedAccountID, contact)
	if err != nil {
		return fmt.Errorf("error adding contact: %w", err)
	}

	i.Logger(ctx).Debug("Adding new contact to the database")
	if err = i.db.addContact(contact); err != nil {
		return fmt.Errorf("error adding contact: %w", err)
	}
	i.Logger(ctx).Info("Successfully added new contact.")
	return nil
}

func (i *impl) validateNewContact(contact Contact) error {
	// Validate account number (must be 10 digits).
	if !regexp.MustCompile(`\A[0-9]{10}\z`).MatchString(contact.AccountNum) {
		return fmt.Errorf("invalid account number: %v", contact.AccountNum)
	}

	// Validate routing number (must be 9 digits).
	if !regexp.MustCompile(`\A[0-9]{9}\z`).MatchString(contact.RoutingNum) {
		return fmt.Errorf("invalid routing number: %v", contact.RoutingNum)
	}

	// Only allow external accounts to deposit.
	if contact.IsExternal && contact.RoutingNum == i.Config().LocalRoutingNum {
		return fmt.Errorf("invalid routing number: contact is marked external but routing number is local")
	}

	// Validate label:
	// Must be >0 and <=30 chars, alphanumeric and spaces, can't start with space.
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 ]{0,29}$`).MatchString(contact.Label) {
		return fmt.Errorf("invalid account label: %v", contact.Label)
	}
	return nil
}

// checkContactAllowed checks if an account is allowed to be created, in which case it returns a nil error.
// Else, returns a non-nil error.
func (i *impl) checkContactAllowed(username, accountID string, contact Contact) error {
	// Don't allow self reference.
	if contact.AccountNum == accountID && contact.RoutingNum == i.Config().LocalRoutingNum {
		return fmt.Errorf("may not add yourself to contacts")
	}

	contacts, err := i.db.getContacts(username)
	if err != nil {
		return err
	}
	// Don't allow duplicate contacts.
	for _, c := range contacts {
		if c.AccountNum == contact.AccountNum &&
			c.RoutingNum == contact.RoutingNum {
			return fmt.Errorf("account already exists as a contact")
		}

		if c.Label == contact.Label {
			return fmt.Errorf("contact already exists with that label")
		}
	}
	return nil
}
