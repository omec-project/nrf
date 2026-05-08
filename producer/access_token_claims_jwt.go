// SPDX-FileCopyrightText: 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/omec-project/openapi/v2/models"
)

type accessTokenJWTClaims struct {
	models.AccessTokenClaims
}

func (c accessTokenJWTClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.Exp == 0 {
		return nil, nil
	}

	return jwt.NewNumericDate(time.Unix(int64(c.Exp), 0)), nil
}

func (c accessTokenJWTClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	return nil, nil
}

func (c accessTokenJWTClaims) GetNotBefore() (*jwt.NumericDate, error) {
	return nil, nil
}

func (c accessTokenJWTClaims) GetIssuer() (string, error) {
	return c.Iss, nil
}

func (c accessTokenJWTClaims) GetSubject() (string, error) {
	return c.Sub, nil
}

func (c accessTokenJWTClaims) GetAudience() (jwt.ClaimStrings, error) {
	if c.Aud.ArrayOfString != nil {
		return jwt.ClaimStrings(*c.Aud.ArrayOfString), nil
	}

	if c.Aud.NFType != nil {
		return jwt.ClaimStrings{string(*c.Aud.NFType)}, nil
	}

	return nil, nil
}
