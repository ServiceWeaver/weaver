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

package emailservice

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"html/template"

	weaver "github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/online_boutique/types"
)

var (
	//go:embed templates/confirmation.html
	tmplData string

	tmpl = template.Must(template.New("email").
		Funcs(template.FuncMap{
			"div": func(x, y int32) int32 { return x / y },
		}).
		Parse(tmplData))
)

type T interface {
	SendOrderConfirmation(ctx context.Context, email string, order types.Order) error
}

type impl struct {
	weaver.Implements[T]
}

// SendOrderConfirmation sends the confirmation email for the order to the
// given email address.
func (s *impl) SendOrderConfirmation(ctx context.Context, email string, order types.Order) error {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, order); err != nil {
		return err
	}
	confirmation := buf.String()
	return s.sendEmail(email, confirmation)
}

func (s *impl) sendEmail(email, confirmation string) error {
	s.Logger().Info(fmt.Sprintf(
		"A request to send email confirmation to %s has been received:\n%s",
		email, confirmation))
	return nil
}
