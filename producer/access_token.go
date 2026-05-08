// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"net/http"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
)

func HandleAccessTokenRequest(request *httpwrapper.Request) *httpwrapper.Response {
	// Param of AccessTokenRsp
	logger.AccessTokenLog.Infoln("Handle AccessTokenRequest")

	accessTokenReq := request.Body.(models.AccessTokenReq)

	response, errResponse := AccessTokenProcedure(accessTokenReq)

	if response != nil {
		// status code is based on SPEC, and option headers
		return httpwrapper.NewResponse(http.StatusOK, nil, response)
	} else if errResponse != nil {
		return httpwrapper.NewResponse(http.StatusBadRequest, nil, errResponse)
	}
	problemDetails := utils.ProblemDetailsUnspecified()
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

func AccessTokenProcedure(request models.AccessTokenReq) (response *models.AccessTokenRsp,
	errResponse *models.AccessTokenErr,
) {
	logger.AccessTokenLog.Infoln("In AccessTokenProcedure")

	var expirationSeconds int32 = 1000
	scope := request.Scope
	tokenType := "Bearer"
	expiresAt := time.Now().Add(time.Duration(expirationSeconds) * time.Second)

	aud := models.AccessTokenClaimsAud{
		ArrayOfString: &[]string{request.GetTargetNfInstanceId()},
	}

	// Create AccessToken
	accessTokenClaims := models.AccessTokenClaims{
		Iss:   "1234567",            // TODO: NF instance id of the NRF
		Sub:   request.NfInstanceId, // nfInstanceId of service consumer
		Aud:   aud,                  // nfInstanceId of service producer
		Scope: scope,                // TODO: the name of the NF services for which the
		Exp:   int32(expiresAt.Unix()),
	}

	mySigningKey := []byte("NRF") // AllYourBase
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, accessTokenJWTClaims{AccessTokenClaims: accessTokenClaims})
	accessToken, err := token.SignedString(mySigningKey)
	if err != nil {
		logger.AccessTokenLog.Warnln("Signed string error: ", err)
		errResponse = &models.AccessTokenErr{
			Error: "invalid_request",
		}

		return nil, errResponse
	}

	response = models.NewAccessTokenRsp(accessToken, tokenType)
	response.SetExpiresIn(expirationSeconds)
	response.SetScope(scope)

	return response, nil
}
