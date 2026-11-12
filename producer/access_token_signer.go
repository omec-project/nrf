// Copyright (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	jwt "github.com/golang-jwt/jwt/v5"
)

const minRSAKeyBits = 2048

// accessTokenSigner holds the NRF private key used to sign access tokens
// (JWS, TS 33.501 §13.4.1.1.2). The matching public key / NRF certificate is
// what producers use to verify tokens, so a producer cannot mint tokens.
type accessTokenSigner struct {
	method jwt.SigningMethod
	key    any
	kid    string
}

var currentAccessTokenSigner atomic.Pointer[accessTokenSigner]

// LoadAccessTokenSigner reads a PEM private key (PKCS#8, PKCS#1 RSA or SEC1 EC)
// from path and installs it as the access token signing key. It must succeed
// before the NRF serves requests; without a key no token is issued.
func LoadAccessTokenSigner(path string) error {
	if path == "" {
		return errors.New("no access token signing key configured")
	}

	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read access token signing key: %w", err)
	}

	signer, err := newAccessTokenSigner(pemBytes)
	if err != nil {
		return fmt.Errorf("load access token signing key %s: %w", path, err)
	}

	currentAccessTokenSigner.Store(signer)

	return nil
}

func newAccessTokenSigner(pemBytes []byte) (*accessTokenSigner, error) {
	var block *pem.Block
	rest := pemBytes
	for {
		block, rest = pem.Decode(rest)
		if block == nil {
			return nil, errors.New("no PEM private key found")
		}
		if strings.HasSuffix(block.Type, "PRIVATE KEY") {
			break
		}
	}

	var (
		key any
		err error
	)
	switch block.Type {
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q", block.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	signer := &accessTokenSigner{key: key}
	var pub any
	switch k := key.(type) {
	case *rsa.PrivateKey:
		if k.N.BitLen() < minRSAKeyBits {
			return nil, fmt.Errorf("RSA key must be at least %d bits", minRSAKeyBits)
		}
		signer.method = jwt.SigningMethodRS256
		pub = &k.PublicKey
	case *ecdsa.PrivateKey:
		switch k.Curve {
		case elliptic.P256():
			signer.method = jwt.SigningMethodES256
		case elliptic.P384():
			signer.method = jwt.SigningMethodES384
		case elliptic.P521():
			signer.method = jwt.SigningMethodES512
		default:
			return nil, errors.New("unsupported ECDSA curve")
		}
		pub = &k.PublicKey
	case ed25519.PrivateKey:
		signer.method = jwt.SigningMethodEdDSA
		pub = k.Public()
	default:
		return nil, fmt.Errorf("unsupported private key type %T", key)
	}

	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}
	sum := sha256.Sum256(der)
	signer.kid = base64.RawURLEncoding.EncodeToString(sum[:])

	return signer, nil
}

// sign returns the compact JWS for claims, with the key id in the header.
func (s *accessTokenSigner) sign(claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(s.method, claims)
	token.Header["kid"] = s.kid

	return token.SignedString(s.key)
}
