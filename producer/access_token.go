// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"errors"
	"net/http"
	"strings"
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

	response, errResponse, err := accessTokenProcedure(accessTokenReq)
	if err != nil {
		logger.AccessTokenLog.Errorln("AccessTokenProcedure failed:", err)
		problemDetails := utils.ProblemDetailsSystemFailure(err.Error())
		return httpwrapper.NewResponse(http.StatusInternalServerError, nil, problemDetails)
	}

	if response != nil {
		// status code is based on SPEC, and option headers
		return httpwrapper.NewResponse(http.StatusOK, nil, response)
	} else if errResponse != nil {
		return httpwrapper.NewResponse(http.StatusBadRequest, nil, errResponse)
	}
	problemDetails := utils.ProblemDetailsUnspecified()
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

// AccessTokenProcedure is kept with its original two-value return signature
// for source compatibility with any external caller of this exported
// function. HandleAccessTokenRequest, the only caller within this package,
// uses accessTokenProcedure instead, which distinguishes a system fault
// (e.g. a DB error while validating requesterFqdn) from an ordinary policy
// rejection so it can be reported as a 500 rather than a 400; that
// distinction has no equivalent in this two-value signature, so a system
// fault surfaces here only as a nil response and nil errResponse.
func AccessTokenProcedure(request models.AccessTokenReq) (response *models.AccessTokenRsp, errResponse *models.AccessTokenErr) {
	response, errResponse, err := accessTokenProcedure(request)
	if err != nil {
		logger.AccessTokenLog.Errorln("AccessTokenProcedure failed:", err)
	}
	return response, errResponse
}

func accessTokenProcedure(request models.AccessTokenReq) (response *models.AccessTokenRsp,
	errResponse *models.AccessTokenErr, err error,
) {
	logger.AccessTokenLog.Infoln("In AccessTokenProcedure")

	errResponse, err = validateRequesterFqdn(request)
	if err != nil {
		return nil, nil, err
	}
	if errResponse != nil {
		return nil, errResponse, nil
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
	accessToken, signErr := token.SignedString(mySigningKey)
	if signErr != nil {
		logger.AccessTokenLog.Warnln("Signed string error: ", signErr)
		errResponse = &models.AccessTokenErr{
			Error: "invalid_request",
		}

		return nil, errResponse, nil
	}

	response = models.NewAccessTokenRsp(accessToken, tokenType)
	response.SetExpiresIn(expirationSeconds)
	response.SetScope(scope)

	return response, nil, nil
}

// errRequesterFqdnValidationUnavailable marks a token request denied because
// validateRequesterFqdn could not determine whether allowedNfDomains permits
// requesterFqdn (a system fault: no DB client, a lookup error, or an
// undecodable stored profile), as opposed to determining that it does not (an
// ordinary policy rejection). HandleAccessTokenRequest maps this to 500, not
// the 400 used for a policy rejection, so a database outage is not reported
// to the client as if the request itself were invalid.
var errRequesterFqdnValidationUnavailable = errors.New("requesterFqdn validation could not be completed")

// validateRequesterFqdn implements the TS 29.510 clause 6.3.5.2.2 check: when
// requesterFqdn is provided, the NRF may validate that the requester NF
// service consumer is allowed to access the target NF Service Producer via
// the producer's allowedNfDomains (clause 6.1.6.2.2). Only the absence of
// requesterFqdn/targetNfInstanceId, or confirming the target profile does
// not exist, skips this optional check; a fault that prevents completing it
// (no DB client, a lookup error, or an undecodable stored profile) fails
// closed instead of silently granting access a policy that could not be
// checked, returned via errRequesterFqdnValidationUnavailable rather than an
// ordinary *models.AccessTokenErr so the caller can tell the two apart.
func validateRequesterFqdn(request models.AccessTokenReq) (*models.AccessTokenErr, error) {
	requesterFqdn, ok := request.GetRequesterFqdnOk()
	if !ok || *requesterFqdn == "" {
		return nil, nil
	}
	targetNfInstanceId, ok := request.GetTargetNfInstanceIdOk()
	if !ok || *targetNfInstanceId == "" {
		return nil, nil
	}

	if dbadapter.DBClient == nil {
		logger.AccessTokenLog.Errorln("requesterFqdn validation unavailable: DB client not initialized")
		return nil, errRequesterFqdnValidationUnavailable
	}
	raw, err := dbadapter.DBClient.RestfulAPIGetOne(collNfProfile, bson.M{fieldNfInstanceId: *targetNfInstanceId})
	if err != nil {
		logger.AccessTokenLog.Errorf("target NF profile lookup failed for requesterFqdn validation: %v", err)
		return nil, errRequesterFqdnValidationUnavailable
	}
	if raw == nil {
		return nil, nil
	}

	decoded, err := util.Decode([]map[string]any{raw}, time.RFC3339)
	if err != nil || len(decoded) == 0 {
		logger.AccessTokenLog.Errorf("target NF profile decode failed for requesterFqdn validation: %v", err)
		return nil, errRequesterFqdnValidationUnavailable
	}

	if requestedServicesAllowFqdn(decoded[0], request.Scope, *requesterFqdn) {
		return nil, nil
	}

	logger.AccessTokenLog.Warnf("requesterFqdn %q is not allowed to access target NF instance %q", *requesterFqdn, *targetNfInstanceId)
	errResponse := models.NewAccessTokenErr("invalid_request")
	errResponse.SetErrorDescription("requesterFqdn is not allowed to access the target NF Service Producer")
	return errResponse, nil
}

// requestedServicesAllowFqdn reports whether requesterFqdn is allowed by the
// NF services actually named in scope (the space-separated list of NF
// service names per TS 29.510 clause 6.3.5.2.2), per each service's
// allowedNfDomains (clause 6.1.6.2.2). Checking only the requested services,
// rather than any service in the profile, prevents an unrelated unrestricted
// service from authorizing access to a service that restricts allowedNfDomains.
// If scope is empty, requesterFqdn validation falls back to checking all of
// the profile's services. Every name in scope must match at least one
// service in profile and be allowed; a name with no matching service in
// profile causes rejection, since falling back to unrelated services could
// let requests for an unknown or misspelled service name be authorized by an
// unrestricted, unrelated service. A name may match multiple entries when the
// profile carries the same service in both nfServices and nfServiceList
// (alternative representations of the same data per TS 29.510 clause
// 6.1.6.2.2); the name is allowed if any matching entry allows it, not only
// if every matching entry does. Only services with NfServiceStatus
// REGISTERED are matched against a requested name, so a service withdrawn
// from the profile cannot authorize access via a stale, unrestricted
// allowedNfDomains policy; this mirrors buildFilter's service-name discovery
// query, which likewise requires NfServiceStatus REGISTERED when a specific
// service name is requested. This is unlike the empty-scope fallback
// (anyNFServiceAllowsFqdn), which deliberately does not filter by
// registration status, consistent with the MongoDB-backed discovery
// predicate for requester-nfinstance-fqdn without a service-name scope.
func requestedServicesAllowFqdn(profile models.NFProfileDiscovery, scope, requesterFqdn string) bool {
	requestedServiceNames := strings.Fields(scope)
	if len(requestedServiceNames) == 0 {
		return anyNFServiceAllowsFqdn(profile, requesterFqdn)
	}

	services := registeredNFServices(profile)
	for _, name := range requestedServiceNames {
		matched := false
		allowed := false
		for _, service := range services {
			if string(service.ServiceName) != name {
				continue
			}
			matched = true
			allowedDomains, ok := service.GetAllowedNfDomainsOk()
			if !ok {
				allowed = true
				continue
			}
			for _, pattern := range allowedDomains {
				if matchesAllowedNfDomainPattern(pattern, requesterFqdn) {
					allowed = true
					break
				}
			}
		}
		if !matched || !allowed {
			return false
		}
	}
	return true
}
