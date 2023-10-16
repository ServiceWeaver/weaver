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

package userservice

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/ServiceWeaver/weaver"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// User contains data pertaining to a user.
type User struct {
	weaver.AutoMarshal
	AccountID string `gorm:"column:accountid;primary_key"`
	Username  string `gorm:"unique;not null"`
	Passhash  []byte `gorm:"not null"`
	Firstname string `gorm:"not null"`
	Lastname  string `gorm:"not null"`
	Birthday  string `gorm:"not null"`
	Timezone  string `gorm:"not null"`
	Address   string `gorm:"not null"`
	State     string `gorm:"not null"`
	Zip       string `gorm:"not null"`
	SSN       string `gorm:"not null"`
}

type userDB struct {
	db *gorm.DB
}

func newUserDB(uri string) (*userDB, error) {
	db, err := gorm.Open(postgres.Open(uri))
	if err != nil {
		return nil, err
	}
	return &userDB{db: db}, nil
}

func (udb *userDB) addUser(user User) error {
	return udb.db.Create(&user).Error
}

// Generates a globally unique alphanumerical accountid.
func (udb *userDB) generateAccountID() string {
	var accountID string
	for {
		accountID = fmt.Sprint(rand.Int63n(1e10-1e9) + 1e9)
		var user User
		err := udb.db.Where("accountid = ?", accountID).First(&user).Error
		// Break if a non-existant account_id has been generated. Else, try again.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			break
		}
	}
	return accountID
}

func (udb *userDB) getUser(username string) (*User, error) {
	var user User
	err := udb.db.Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}
