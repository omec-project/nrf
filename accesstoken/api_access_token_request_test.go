// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package accesstoken_test

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/omec-project/nrf/accesstoken"
	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/openapi/v2/Nnrf_AccessToken"
	"github.com/omec-project/openapi/v2/models"
)

func TestAccessTokenRequest(t *testing.T) {
	// run accesstoken Server Routine
	go func() {
		kl, _ := os.OpenFile("/home/sslkey.log", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		router := accesstoken.NewRouter()

		server := http.Server{
			Addr: factory.NRF_DEFAULT_IPV4 + ":" + strconv.Itoa(factory.NRF_DEFAULT_PORT),
			TLSConfig: &tls.Config{
				KeyLogWriter: kl,
			},

			Handler: router,
		}
		_ = server.ListenAndServeTLS("/var/run/certs/tls.crt", "/var/run/certs/tls.key")
	}()
	time.Sleep(time.Duration(2) * time.Second)

	// connect to mongoDB
	dbadapter.ConnectToDBClient("aether", "mongodb://140.113.214.205:30030", false, false)

	// Set client and set url
	configuration := Nnrf_AccessToken.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["nrfApiRoot"]; exists {
		apiRootVar.DefaultValue = "https://" + factory.NRF_DEFAULT_IPV4 + ":" + strconv.Itoa(factory.NRF_DEFAULT_PORT)
		serverConfig.Variables["nrfApiRoot"] = apiRootVar
	}

	client := Nnrf_AccessToken.NewAPIClient(configuration)
	apiAccessTokenRequestRequest := client.AccessTokenRequestAPI.AccessTokenRequest(context.TODO())
	apiAccessTokenRequestRequest = apiAccessTokenRequestRequest.NfType(models.NFTYPE_NRF)
	apiAccessTokenRequestRequest = apiAccessTokenRequestRequest.TargetNfType(models.NFTYPE_NRF)
	apiAccessTokenRequestRequest = apiAccessTokenRequestRequest.TargetNfInstanceId("2")
	requesterPlmn := models.NewPlmnId("111", "111")
	apiAccessTokenRequestRequest = apiAccessTokenRequestRequest.RequesterPlmn(*requesterPlmn)
	apiAccessTokenRequestRequest = apiAccessTokenRequestRequest.TargetPlmn(*requesterPlmn)
	apiAccessTokenRequestRequest = apiAccessTokenRequestRequest.GrantType("client_credentials") // Set test data (with expected data)
	apiAccessTokenRequestRequest = apiAccessTokenRequestRequest.NfInstanceId("0")               // Set test data (with expected data)
	apiAccessTokenRequestRequest = apiAccessTokenRequestRequest.Scope("nnrf-nfm")               // Set test data (with expected data)

	// Check test data (Use RESTful GET)
	rep, res, err := client.AccessTokenRequestAPI.AccessTokenRequestExecute(apiAccessTokenRequestRequest)
	if err != nil {
		logger.AppLog.Errorln(err)
	}
	if res != nil {
		if status := res.StatusCode; status != http.StatusOK {
			logger.AppLog.Errorf("handler returned wrong status code: got %v want %v",
				status, http.StatusOK)
		}
	}

	t.Logf("%+v", rep)
}
