// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"net/http"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/nrf/util"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
	"go.mongodb.org/mongo-driver/v2/bson"
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

	if errResponse = validateRequesterFqdn(request); errResponse != nil {
		return nil, errResponse
	}

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

// validateRequesterFqdn implements the TS 29.510 clause 6.3.5.2.2 check: when
// requesterFqdn is provided, the NRF may validate that the requester NF
// service consumer is allowed to access the target NF Service Producer via
// the producer's allowedNfDomains (clause 6.1.6.2.2). Lookup failures fail
// open since this check is optional ("may"), not mandatory, per spec.
func validateRequesterFqdn(request models.AccessTokenReq) *models.AccessTokenErr {
	requesterFqdn, ok := request.GetRequesterFqdnOk()
	if !ok || *requesterFqdn == "" {
		return nil
	}
	targetNfInstanceId, ok := request.GetTargetNfInstanceIdOk()
	if !ok || *targetNfInstanceId == "" {
		return nil
	}

	raw, err := dbadapter.DBClient.RestfulAPIGetOne(collNfProfile, bson.M{fieldNfInstanceId: *targetNfInstanceId})
	if err != nil {
		logger.AccessTokenLog.Warnf("target NF profile lookup failed for requesterFqdn validation: %v", err)
		return nil
	}
	if raw == nil {
		return nil
	}

	decoded, err := util.Decode([]map[string]any{raw}, time.RFC3339)
	if err != nil || len(decoded) == 0 {
		logger.AccessTokenLog.Warnf("target NF profile decode failed for requesterFqdn validation: %v", err)
		return nil
	}

	if anyNFServiceAllowsFqdn(decoded[0], *requesterFqdn) {
		return nil
	}

	logger.AccessTokenLog.Warnf("requesterFqdn %q is not allowed to access target NF instance %q", *requesterFqdn, *targetNfInstanceId)
	errResponse := models.NewAccessTokenErr("invalid_request")
	errResponse.SetErrorDescription("requesterFqdn is not allowed to access the target NF Service Producer")
	return errResponse
}
