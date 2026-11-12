// Copyright (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"crypto/ecdsa"
	"crypto/ed25519"
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

func installTestSigner(t *testing.T, pemBytes []byte) {
	t.Helper()
	signer, err := newAccessTokenSigner(pemBytes)
	if err != nil {
		t.Fatalf("newAccessTokenSigner: %v", err)
	}
	original := currentAccessTokenSigner.Load()
	currentAccessTokenSigner.Store(signer)
	t.Cleanup(func() { currentAccessTokenSigner.Store(original) })
}

// must returns key or panics, so key generation can be used inline in tables.
func must[K any](key K, err error) K {
	if err != nil {
		panic(err)
	}
	return key
}

func TestAccessTokenIsSignedAsymmetricallyAndVerifiable(t *testing.T) {
	tests := []struct {
		name string
		alg  string
		pem  func(t *testing.T) ([]byte, any)
	}{
		{"RSA PKCS#8", jwt.SigningMethodRS256.Alg(), func(t *testing.T) ([]byte, any) {
			k := must(rsa.GenerateKey(rand.Reader, 2048))
			return pkcs8PEM(t, k), &k.PublicKey
		}},
		{"RSA PKCS#1", jwt.SigningMethodRS256.Alg(), func(t *testing.T) ([]byte, any) {
			k := must(rsa.GenerateKey(rand.Reader, 2048))
			return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}), &k.PublicKey
		}},
		{"ECDSA P-256 PKCS#8", jwt.SigningMethodES256.Alg(), func(t *testing.T) ([]byte, any) {
			k := must(ecdsa.GenerateKey(elliptic.P256(), rand.Reader))
			return pkcs8PEM(t, k), &k.PublicKey
		}},
		{"ECDSA P-256 SEC1", jwt.SigningMethodES256.Alg(), func(t *testing.T) ([]byte, any) {
			k := must(ecdsa.GenerateKey(elliptic.P256(), rand.Reader))
			der, err := x509.MarshalECPrivateKey(k)
			if err != nil {
				t.Fatalf("marshal EC key: %v", err)
			}
			return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), &k.PublicKey
		}},
		{"ECDSA P-384 PKCS#8", jwt.SigningMethodES384.Alg(), func(t *testing.T) ([]byte, any) {
			k := must(ecdsa.GenerateKey(elliptic.P384(), rand.Reader))
			return pkcs8PEM(t, k), &k.PublicKey
		}},
		{"ECDSA P-521 PKCS#8", jwt.SigningMethodES512.Alg(), func(t *testing.T) ([]byte, any) {
			k := must(ecdsa.GenerateKey(elliptic.P521(), rand.Reader))
			return pkcs8PEM(t, k), &k.PublicKey
		}},
		{"Ed25519 PKCS#8", jwt.SigningMethodEdDSA.Alg(), func(t *testing.T) ([]byte, any) {
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatalf("generate key: %v", err)
			}
			return pkcs8PEM(t, priv), pub
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pemBytes, pub := tc.pem(t)
			installTestSigner(t, pemBytes)

			req := models.NewAccessTokenReqWithDefaults()
			req.NfInstanceId = "consumer-1"
			req.Scope = testServiceNameNudmEe
			req.SetTargetNfInstanceId(testTargetNfInstanceId)

			rsp, errRsp, err := accessTokenProcedure(*req)
			if err != nil || errRsp != nil || rsp == nil {
				t.Fatalf("unexpected result rsp=%+v errRsp=%+v err=%v", rsp, errRsp, err)
			}

			parsed, err := jwt.Parse(rsp.AccessToken, func(*jwt.Token) (any, error) {
				return pub, nil
			}, jwt.WithValidMethods([]string{tc.alg}))
			if err != nil || !parsed.Valid {
				t.Fatalf("token did not verify with the NRF public key under %s: %v", tc.alg, err)
			}
			if kid, _ := parsed.Header["kid"].(string); kid == "" {
				t.Fatal("expected kid header")
			}
			sub, err := parsed.Claims.GetSubject()
			if err != nil || sub != "consumer-1" {
				t.Fatalf("unexpected sub %q (err=%v)", sub, err)
			}
		})
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
	}, jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}))
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
	if s := currentAccessTokenSigner.Load(); s == nil || s.method.Alg() != jwt.SigningMethodES384.Alg() {
		t.Fatalf("expected ES384 signer, got %+v", s)
	}
}
