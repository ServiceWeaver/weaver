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

package certs_test

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/internal/tool/certs"
	"github.com/google/go-cmp/cmp"
)

func TestGenerateCACert(t *testing.T) {
	caCert, _, err := certs.GenerateCACert()
	if err != nil {
		t.Fatal(err)
	}

	if caCert.SignatureAlgorithm != x509.SHA256WithRSA {
		t.Errorf("want RSA certificate, got %s", caCert.SignatureAlgorithm)
	}
	if time.Now().After(caCert.NotAfter) {
		t.Errorf("certificate expired")
	}
	if diff := cmp.Diff([]string{"ca"}, caCert.DNSNames); diff != "" {
		t.Errorf("unexpected ca certificate names. (-want +got): %s", diff)
	}
}

func TestGenerateSignedCert(t *testing.T) {
	caCert, caKey, err := certs.GenerateCACert()
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := certs.GenerateSignedCert(caCert, caKey, "name1", "name2")
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]string{"name1", "name2"}, cert.DNSNames); diff != "" {
		t.Errorf("unexpected leaf certificate names. (-want +got): %s", diff)
	}
}

func TestVerifySignedCert(t *testing.T) {
	caCert, caKey, err := certs.GenerateCACert()
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := certs.GenerateSignedCert(caCert, caKey, "name1", "name2")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the certificate.
	names, err := certs.VerifySignedCert(cert.Raw, caCert)
	if err != nil {
		t.Errorf("cannot verify certificate: %v", err)
	}
	if diff := cmp.Diff([]string{"name1", "name2"}, names); diff != "" {
		t.Errorf("unexpected certificate names. (-want +got): %s", diff)
	}
}
