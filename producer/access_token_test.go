// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"errors"
	"testing"

	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/openapi/v2/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	testTargetNfInstanceId = "target-nf-1"
	testServiceNameNudmEe  = "nudm-ee"
	testOtherFqdn          = "other.example.com"
)

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

// TestValidateRequesterFqdnFailsClosedWhenDBClientNil verifies that
// validation denies the request, rather than panicking or silently granting
// access, when dbadapter.DBClient has not been initialized: this is a fault
// that prevents checking allowedNfDomains, not a case where no restriction
// applies.
func TestValidateRequesterFqdnFailsClosedWhenDBClientNil(t *testing.T) {
	original := dbadapter.DBClient
	dbadapter.DBClient = nil
	t.Cleanup(func() { dbadapter.DBClient = original })

	req := models.NewAccessTokenReqWithDefaults()
	req.SetTargetNfInstanceId(testTargetNfInstanceId)
	req.SetRequesterFqdn(testExampleFqdn)
	errResp := validateRequesterFqdn(*req)
	if errResp == nil {
		t.Fatal("expected an error when DBClient is nil, since allowedNfDomains cannot be checked")
	}
}

// erroringAccessTokenDBClient simulates a database lookup failure (e.g. a
// transient network error) for the target NF profile.
type erroringAccessTokenDBClient struct {
	dbadapter.DBInterface
}

func (db *erroringAccessTokenDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	return nil, errors.New("simulated database failure")
}

// TestValidateRequesterFqdnFailsClosedOnLookupError verifies that a database
// error while fetching the target NF profile denies the request instead of
// silently granting access a policy that could not be checked.
func TestValidateRequesterFqdnFailsClosedOnLookupError(t *testing.T) {
	original := dbadapter.DBClient
	dbadapter.DBClient = &erroringAccessTokenDBClient{}
	t.Cleanup(func() { dbadapter.DBClient = original })

	req := models.NewAccessTokenReqWithDefaults()
	req.SetTargetNfInstanceId(testTargetNfInstanceId)
	req.SetRequesterFqdn(testExampleFqdn)
	errResp := validateRequesterFqdn(*req)
	if errResp == nil {
		t.Fatal("expected an error when the target NF profile lookup fails")
	}
}

// TestValidateRequesterFqdnFailsClosedOnUndecodableProfile verifies that a
// stored profile which fails to decode denies the request instead of
// silently granting access a policy that could not be checked.
func TestValidateRequesterFqdnFailsClosedOnUndecodableProfile(t *testing.T) {
	withMockAccessTokenDB(t, map[string]interface{}{
		fieldNfInstanceId: testTargetNfInstanceId,
		fieldNfTypeLower:  nfTypeUDM,
		testFieldNfStatus: nfServiceStatusRegistered,
		// priority out of the [0, 65535] range TS 29.510 requires makes
		// util.Decode reject the whole profile.
		"priority": 999999,
	})

	req := models.NewAccessTokenReqWithDefaults()
	req.SetTargetNfInstanceId(testTargetNfInstanceId)
	req.SetRequesterFqdn(testExampleFqdn)
	errResp := validateRequesterFqdn(*req)
	if errResp == nil {
		t.Fatal("expected an error when the target NF profile fails to decode")
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
			fieldAllowedNfDomains: []string{testOtherFqdn},
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

// TestValidateRequesterFqdnScopedToRestrictedServiceIsRejected verifies that
// an unrelated, unrestricted service in the target profile cannot authorize
// access to a service named in scope whose allowedNfDomains restricts the
// requesterFqdn: only services named in scope are evaluated.
func TestValidateRequesterFqdnScopedToRestrictedServiceIsRejected(t *testing.T) {
	withMockAccessTokenDB(t, map[string]interface{}{
		fieldNfInstanceId: testTargetNfInstanceId,
		fieldNfTypeLower:  nfTypeUDM,
		testFieldNfStatus: nfServiceStatusRegistered,
		fieldNfServices: []map[string]interface{}{
			{
				fieldServiceName:      testServiceNameNudmSdm,
				fieldNfServiceStatus:  nfServiceStatusRegistered,
				fieldAllowedNfDomains: []string{testOtherFqdn},
			},
			{
				fieldServiceName:     testServiceNameNudmEe,
				fieldNfServiceStatus: nfServiceStatusRegistered,
			},
		},
	})

	req := models.NewAccessTokenReqWithDefaults()
	req.SetTargetNfInstanceId(testTargetNfInstanceId)
	req.SetRequesterFqdn(testExampleFqdn)
	req.Scope = testServiceNameNudmSdm
	errResp := validateRequesterFqdn(*req)
	if errResp == nil {
		t.Fatal("expected an error: the requested service restricts allowedNfDomains, regardless of an unrelated service's policy")
	}
}

// TestValidateRequesterFqdnScopedToUnrestrictedServiceIsAllowed verifies that
// a restriction on a service not named in scope does not block a request for
// a different, unrestricted service.
func TestValidateRequesterFqdnScopedToUnrestrictedServiceIsAllowed(t *testing.T) {
	withMockAccessTokenDB(t, map[string]interface{}{
		fieldNfInstanceId: testTargetNfInstanceId,
		fieldNfTypeLower:  nfTypeUDM,
		testFieldNfStatus: nfServiceStatusRegistered,
		fieldNfServices: []map[string]interface{}{
			{
				fieldServiceName:      testServiceNameNudmSdm,
				fieldNfServiceStatus:  nfServiceStatusRegistered,
				fieldAllowedNfDomains: []string{testOtherFqdn},
			},
			{
				fieldServiceName:     testServiceNameNudmEe,
				fieldNfServiceStatus: nfServiceStatusRegistered,
			},
		},
	})

	req := models.NewAccessTokenReqWithDefaults()
	req.SetTargetNfInstanceId(testTargetNfInstanceId)
	req.SetRequesterFqdn(testExampleFqdn)
	req.Scope = testServiceNameNudmEe
	if errResp := validateRequesterFqdn(*req); errResp != nil {
		t.Fatalf("expected no error: the requested service has no allowedNfDomains restriction, got %+v", errResp)
	}
}

// TestValidateRequesterFqdnRejectsUnmatchedScope verifies that a scope naming
// no service in the profile is rejected rather than falling back to check
// unrelated services, so an unknown or misspelled scope cannot be authorized
// by an unrestricted service unrelated to the request.
func TestValidateRequesterFqdnRejectsUnmatchedScope(t *testing.T) {
	withMockAccessTokenDB(t, map[string]interface{}{
		fieldNfInstanceId: testTargetNfInstanceId,
		fieldNfTypeLower:  nfTypeUDM,
		testFieldNfStatus: nfServiceStatusRegistered,
		fieldNfServices: []map[string]interface{}{
			{
				fieldServiceName:      testServiceNameNudmSdm,
				fieldNfServiceStatus:  nfServiceStatusRegistered,
				fieldAllowedNfDomains: []string{testOtherFqdn},
			},
			{
				fieldServiceName:     testServiceNameNudmEe,
				fieldNfServiceStatus: nfServiceStatusRegistered,
			},
		},
	})

	req := models.NewAccessTokenReqWithDefaults()
	req.SetTargetNfInstanceId(testTargetNfInstanceId)
	req.SetRequesterFqdn(testExampleFqdn)
	req.Scope = "unknown-service"
	errResp := validateRequesterFqdn(*req)
	if errResp == nil {
		t.Fatal("expected an error: scope names no service in the profile, so an unrelated unrestricted service must not authorize access")
	}
}

// TestValidateRequesterFqdnAllowsWhenDuplicateServiceEntryAllows verifies
// that when a service name is advertised via both the deprecated nfServices
// array and its TS 29.510 Rel-16 replacement, nfServiceList (alternative
// representations of the same data per clause 6.1.6.2.2), requesterFqdn is
// allowed if any matching entry allows it, even though another matching entry
// restricts allowedNfDomains to a different domain.
func TestValidateRequesterFqdnAllowsWhenDuplicateServiceEntryAllows(t *testing.T) {
	withMockAccessTokenDB(t, map[string]interface{}{
		fieldNfInstanceId: testTargetNfInstanceId,
		fieldNfTypeLower:  nfTypeUDM,
		testFieldNfStatus: nfServiceStatusRegistered,
		fieldNfServices: []map[string]interface{}{
			{
				fieldServiceName:      testServiceNameNudmSdm,
				fieldNfServiceStatus:  nfServiceStatusRegistered,
				fieldAllowedNfDomains: []string{testOtherFqdn},
			},
		},
		fieldNfServiceList: map[string]interface{}{
			testServiceInstanceId: map[string]interface{}{
				fieldServiceName:     testServiceNameNudmSdm,
				fieldNfServiceStatus: nfServiceStatusRegistered,
			},
		},
	})

	req := models.NewAccessTokenReqWithDefaults()
	req.SetTargetNfInstanceId(testTargetNfInstanceId)
	req.SetRequesterFqdn(testExampleFqdn)
	req.Scope = testServiceNameNudmSdm
	if errResp := validateRequesterFqdn(*req); errResp != nil {
		t.Fatalf("expected no error: the nfServiceList entry for the duplicated service has no allowedNfDomains restriction, got %+v", errResp)
	}
}

// TestValidateRequesterFqdnRejectsScopeMixingKnownAndUnknownServices verifies
// that a scope naming both a known, unrestricted service and an unknown
// service is rejected: every name in scope must match a service in the
// profile, so an unknown or misspelled name cannot ride along with a
// legitimate one.
func TestValidateRequesterFqdnRejectsScopeMixingKnownAndUnknownServices(t *testing.T) {
	withMockAccessTokenDB(t, map[string]interface{}{
		fieldNfInstanceId: testTargetNfInstanceId,
		fieldNfTypeLower:  nfTypeUDM,
		testFieldNfStatus: nfServiceStatusRegistered,
		fieldNfServices: []map[string]interface{}{
			{
				fieldServiceName:     testServiceNameNudmSdm,
				fieldNfServiceStatus: nfServiceStatusRegistered,
			},
		},
	})

	req := models.NewAccessTokenReqWithDefaults()
	req.SetTargetNfInstanceId(testTargetNfInstanceId)
	req.SetRequesterFqdn(testExampleFqdn)
	req.Scope = testServiceNameNudmSdm + " unknown-service"
	errResp := validateRequesterFqdn(*req)
	if errResp == nil {
		t.Fatal("expected an error: an unknown service in scope must not be silently ignored because another requested service matched")
	}
}
