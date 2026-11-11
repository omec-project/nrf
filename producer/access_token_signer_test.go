// Copyright (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/omec-project/openapi/v2/models"
)

func pkcs8PEM(t *testing.T, key any) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func installTestSigner(t *testing.T, key *ecdsa.PrivateKey) {
	t.Helper()
	signer, err := newAccessTokenSigner(pkcs8PEM(t, key))
	if err != nil {
		t.Fatalf("newAccessTokenSigner: %v", err)
	}
	original := currentAccessTokenSigner.Load()
	currentAccessTokenSigner.Store(signer)
	t.Cleanup(func() { currentAccessTokenSigner.Store(original) })
}

func TestAccessTokenIsSignedAsymmetricallyAndVerifiable(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	installTestSigner(t, key)

	req := models.NewAccessTokenReqWithDefaults()
	req.NfInstanceId = "consumer-1"
	req.Scope = testServiceNameNudmEe
	req.SetTargetNfInstanceId(testTargetNfInstanceId)

	rsp, errRsp, err := accessTokenProcedure(*req)
	if err != nil || errRsp != nil || rsp == nil {
		t.Fatalf("unexpected result rsp=%+v errRsp=%+v err=%v", rsp, errRsp, err)
	}

	parsed, err := jwt.Parse(rsp.AccessToken, func(*jwt.Token) (any, error) {
		return &key.PublicKey, nil
	}, jwt.WithValidMethods([]string{"ES256"}))
	if err != nil || !parsed.Valid {
		t.Fatalf("token did not verify with the NRF public key: %v", err)
	}
	if parsed.Header["kid"] == "" || parsed.Header["kid"] == nil {
		t.Fatal("expected kid header")
	}
	sub, err := parsed.Claims.GetSubject()
	if err != nil || sub != "consumer-1" {
		t.Fatalf("unexpected sub %q (err=%v)", sub, err)
	}
}

func TestAccessTokenSignedWithLegacyHardCodedSecretIsRejected(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	forged, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "victim"}).
		SignedString([]byte("NRF"))
	if err != nil {
		t.Fatalf("sign forged token: %v", err)
	}

	_, err = jwt.Parse(forged, func(*jwt.Token) (any, error) {
		return &key.PublicKey, nil
	}, jwt.WithValidMethods([]string{"ES256"}))
	if err == nil {
		t.Fatal("token signed with the old hard-coded secret must not verify")
	}
}

func TestAccessTokenProcedureFailsWithoutSigningKey(t *testing.T) {
	original := currentAccessTokenSigner.Load()
	currentAccessTokenSigner.Store(nil)
	t.Cleanup(func() { currentAccessTokenSigner.Store(original) })

	req := models.NewAccessTokenReqWithDefaults()
	rsp, errRsp, err := accessTokenProcedure(*req)
	if err == nil || rsp != nil || errRsp != nil {
		t.Fatalf("expected system error only, got rsp=%+v errRsp=%+v err=%v", rsp, errRsp, err)
	}
}

func TestNewAccessTokenSignerRejectsWeakOrInvalidKeys(t *testing.T) {
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	if _, err := newAccessTokenSigner(pkcs8PEM(t, weak)); err == nil {
		t.Fatal("expected 1024-bit RSA key to be rejected")
	}
	if _, err := newAccessTokenSigner([]byte("not a pem")); err == nil {
		t.Fatal("expected non-PEM input to be rejected")
	}
}

func TestLoadAccessTokenSigner(t *testing.T) {
	original := currentAccessTokenSigner.Load()
	t.Cleanup(func() { currentAccessTokenSigner.Store(original) })

	if err := LoadAccessTokenSigner(""); err == nil {
		t.Fatal("expected error for empty path")
	}
	if err := LoadAccessTokenSigner(filepath.Join(t.TempDir(), "missing.key")); err == nil {
		t.Fatal("expected error for missing file")
	}

	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "nrf.key")
	if err := os.WriteFile(path, pkcs8PEM(t, key), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := LoadAccessTokenSigner(path); err != nil {
		t.Fatalf("LoadAccessTokenSigner: %v", err)
	}
	if s := currentAccessTokenSigner.Load(); s == nil || s.method.Alg() != "ES384" {
		t.Fatalf("expected ES384 signer, got %+v", s)
	}
}
