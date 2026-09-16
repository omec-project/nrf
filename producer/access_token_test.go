// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"testing"

	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/openapi/v2/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const testTargetNfInstanceId = "target-nf-1"

type mockAccessTokenDBClient struct {
	dbadapter.DBInterface
	profile map[string]interface{}
}

func (db *mockAccessTokenDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	if collName == collNfProfile && filter[fieldNfInstanceId] == testTargetNfInstanceId {
		return db.profile, nil
	}
	return nil, nil
}

func withMockAccessTokenDB(t *testing.T, profile map[string]interface{}) {
	t.Helper()
	original := dbadapter.DBClient
	dbadapter.DBClient = &mockAccessTokenDBClient{profile: profile}
	t.Cleanup(func() { dbadapter.DBClient = original })
}

func TestValidateRequesterFqdnSkipsWhenNotProvided(t *testing.T) {
	req := models.NewAccessTokenReqWithDefaults()
	req.SetTargetNfInstanceId(testTargetNfInstanceId)
	if errResp := validateRequesterFqdn(*req); errResp != nil {
		t.Fatalf("expected no error when requesterFqdn is absent, got %+v", errResp)
	}
}

func TestValidateRequesterFqdnSkipsWhenTargetMissing(t *testing.T) {
	req := models.NewAccessTokenReqWithDefaults()
	req.SetRequesterFqdn(testExampleFqdn)
	if errResp := validateRequesterFqdn(*req); errResp != nil {
		t.Fatalf("expected no error when targetNfInstanceId is absent, got %+v", errResp)
	}
}

func TestValidateRequesterFqdnAllowsUnrestrictedProfile(t *testing.T) {
	withMockAccessTokenDB(t, map[string]interface{}{
		fieldNfInstanceId: testTargetNfInstanceId,
		fieldNfTypeLower:  nfTypeUDM,
		testFieldNfStatus: nfServiceStatusRegistered,
		fieldNfServices: []map[string]interface{}{{
			fieldServiceName:     testServiceNameNudmSdm,
			fieldNfServiceStatus: nfServiceStatusRegistered,
		}},
	})

	req := models.NewAccessTokenReqWithDefaults()
	req.SetTargetNfInstanceId(testTargetNfInstanceId)
	req.SetRequesterFqdn(testExampleFqdn)
	if errResp := validateRequesterFqdn(*req); errResp != nil {
		t.Fatalf("expected no error for an unrestricted target profile, got %+v", errResp)
	}
}

func TestValidateRequesterFqdnRejectsDisallowedDomain(t *testing.T) {
	withMockAccessTokenDB(t, map[string]interface{}{
		fieldNfInstanceId: testTargetNfInstanceId,
		fieldNfTypeLower:  nfTypeUDM,
		testFieldNfStatus: nfServiceStatusRegistered,
		fieldNfServices: []map[string]interface{}{{
			fieldServiceName:      testServiceNameNudmSdm,
			fieldNfServiceStatus:  nfServiceStatusRegistered,
			fieldAllowedNfDomains: []string{"other.example.com"},
		}},
	})

	req := models.NewAccessTokenReqWithDefaults()
	req.SetTargetNfInstanceId(testTargetNfInstanceId)
	req.SetRequesterFqdn(testExampleFqdn)
	errResp := validateRequesterFqdn(*req)
	if errResp == nil {
		t.Fatal("expected an error for a requesterFqdn not in allowedNfDomains")
	}
	if errResp.Error != "invalid_request" {
		t.Fatalf("unexpected error code: %q", errResp.Error)
	}
}

func TestValidateRequesterFqdnAllowsMatchingPattern(t *testing.T) {
	withMockAccessTokenDB(t, map[string]interface{}{
		fieldNfInstanceId: testTargetNfInstanceId,
		fieldNfTypeLower:  nfTypeUDM,
		testFieldNfStatus: nfServiceStatusRegistered,
		fieldNfServices: []map[string]interface{}{{
			fieldServiceName:      testServiceNameNudmSdm,
			fieldNfServiceStatus:  nfServiceStatusRegistered,
			fieldAllowedNfDomains: []string{`^.*\.example\.com$`},
		}},
	})

	req := models.NewAccessTokenReqWithDefaults()
	req.SetTargetNfInstanceId(testTargetNfInstanceId)
	req.SetRequesterFqdn("amf1.example.com")
	if errResp := validateRequesterFqdn(*req); errResp != nil {
		t.Fatalf("expected no error for a requesterFqdn matching an allowedNfDomains pattern, got %+v", errResp)
	}
}
