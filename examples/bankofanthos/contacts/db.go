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
	"errors"

	"github.com/ServiceWeaver/weaver"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Contact represents an account's contact details.
type Contact struct {
	weaver.AutoMarshal
	Username   string `gorm:"not null"`
	Label      string `gorm:"not null"`
	AccountNum string `gorm:"not null"`
	RoutingNum string `gorm:"not null"`
	IsExternal bool   `gorm:"not null"`
}

type contactDB struct {
	db *gorm.DB
}

func newContactDB(uri string) (*contactDB, error) {
	db, err := gorm.Open(postgres.Open(uri))
	if err != nil {
		return nil, err
	}
	return &contactDB{db: db}, nil
}

func (cdb *contactDB) addContact(contact Contact) error {
	return cdb.db.Create(&contact).Error
}

func (cdb *contactDB) getContacts(username string) ([]Contact, error) {
	contacts := []Contact{}
	err := cdb.db.Where("username = ?", username).Find(&contacts).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return contacts, nil
}
