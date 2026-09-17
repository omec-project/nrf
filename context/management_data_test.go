// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/openapi/v2/models"
)

const (
	testServiceInstanceId = "svc-0"
	testNfInstanceId      = "test-instance-id"
)

// TestNnrfNFManagementDataModel_NfServiceList verifies that the nfServiceList
// field (the TS 29.510 Rel-16 replacement for the deprecated nfServices array)
// provided by a registering NF is preserved in the stored/returned profile.
func TestNnrfNFManagementDataModel_NfServiceList(t *testing.T) {
	originalNrfConfig := factory.NrfConfig
	t.Cleanup(func() { factory.NrfConfig = originalNrfConfig })
	factory.NrfConfig = factory.Config{Configuration: &factory.Configuration{}}

	nfServiceList := map[string]models.NFService{
		testServiceInstanceId: {
			ServiceInstanceId: testServiceInstanceId,
			ServiceName:       models.SERVICENAME_NUDM_SDM,
			NfServiceStatus:   models.NFSERVICESTATUS_REGISTERED,
		},
	}

	nfprofile := models.NFProfile{
		NfInstanceId:  testNfInstanceId,
		NfType:        models.NFTYPE_UDM,
		NfStatus:      models.NFSTATUS_REGISTERED,
		PlmnList:      []models.PlmnId{{Mcc: "001", Mnc: "01"}},
		NfServiceList: &nfServiceList,
	}

	var nf models.NFProfile
	if err := NnrfNFManagementDataModel(&nf, nfprofile); err != nil {
		t.Fatalf("NnrfNFManagementDataModel returned error: %v", err)
	}

	got, ok := nf.GetNfServiceListOk()
	if !ok || got == nil {
		t.Fatalf("expected nfServiceList to be preserved, got ok=%v", ok)
	}
	svc, ok := (*got)[testServiceInstanceId]
	if !ok {
		t.Fatalf("expected svc-0 entry in nfServiceList, got %+v", *got)
	}
	if svc.GetServiceName() != models.SERVICENAME_NUDM_SDM {
		t.Fatalf("unexpected service name: %q", svc.GetServiceName())
	}
}

// TestNnrfNFManagementDataModelRejectsInvalidAllowedNfDomainsPattern verifies
// that registration is rejected when a service's allowedNfDomains (TS 29.510
// clause 6.1.6.2.2) contains a pattern that fails to compile as a regular
// expression, whether advertised via the deprecated nfServices array or its
// Rel-16 replacement, nfServiceList. Rejecting invalid patterns at
// registration prevents them from later breaking the MongoDB-backed
// discovery query's $regexMatch.
func TestNnrfNFManagementDataModelRejectsInvalidAllowedNfDomainsPattern(t *testing.T) {
	originalNrfConfig := factory.NrfConfig
	t.Cleanup(func() { factory.NrfConfig = originalNrfConfig })
	factory.NrfConfig = factory.Config{Configuration: &factory.Configuration{}}

	baseProfile := models.NFProfile{
		NfInstanceId: testNfInstanceId,
		NfType:       models.NFTYPE_UDM,
		NfStatus:     models.NFSTATUS_REGISTERED,
		PlmnList:     []models.PlmnId{{Mcc: "001", Mnc: "01"}},
	}

	t.Run("nfServices", func(t *testing.T) {
		nfprofile := baseProfile
		nfprofile.NfServices = []models.NFService{{
			ServiceName:     models.SERVICENAME_NUDM_SDM,
			NfServiceStatus: models.NFSERVICESTATUS_REGISTERED,
			AllowedNfDomains: []string{
				"(unclosed",
			},
		}}
		var nf models.NFProfile
		if err := NnrfNFManagementDataModel(&nf, nfprofile); err == nil {
			t.Fatal("expected an error for an invalid allowedNfDomains pattern in nfServices")
		}
	})

	t.Run("nfServiceList", func(t *testing.T) {
		nfprofile := baseProfile
		nfServiceList := map[string]models.NFService{
			testServiceInstanceId: {
				ServiceInstanceId: testServiceInstanceId,
				ServiceName:       models.SERVICENAME_NUDM_SDM,
				NfServiceStatus:   models.NFSERVICESTATUS_REGISTERED,
				AllowedNfDomains: []string{
					"(unclosed",
				},
			},
		}
		nfprofile.NfServiceList = &nfServiceList
		var nf models.NFProfile
		if err := NnrfNFManagementDataModel(&nf, nfprofile); err == nil {
			t.Fatal("expected an error for an invalid allowedNfDomains pattern in nfServiceList")
		}
	})
}

// TestNnrfNFManagementDataModelRejectsReDoSRiskPattern verifies that
// registration is rejected when allowedNfDomains contains a pattern prone to
// catastrophic backtracking under a backtracking engine - valid RE2, but
// exponential (or merely very slow) when later evaluated by MongoDB's
// PCRE-based $regexMatch during discovery: a nested unbounded quantifier
// (e.g. "(a+)+"), a quantifier (bounded or not) over an ambiguous
// alternation (e.g. "(a|aa)+" or "(a|aa){1000}"), a quantifier bound that
// is simply too large regardless of nesting (e.g. "a{1001}"), or an outer
// repeat over a nested optional quantifier (e.g. "(a?a?)+"): although a bare
// "?" cannot itself compound ambiguity by repeating, one nested inside an
// outer repeat still gives a backtracking engine exponentially many ways to
// partition a non-matching input.
func TestNnrfNFManagementDataModelRejectsReDoSRiskPattern(t *testing.T) {
	originalNrfConfig := factory.NrfConfig
	t.Cleanup(func() { factory.NrfConfig = originalNrfConfig })
	factory.NrfConfig = factory.Config{Configuration: &factory.Configuration{}}

	riskyPatterns := []string{"(a+)+", "(a|aa)+", "(a|aa){1000}", "a{1001}", "(a?a?)+"}
	for _, pattern := range riskyPatterns {
		t.Run(pattern, func(t *testing.T) {
			nfprofile := models.NFProfile{
				NfInstanceId: testNfInstanceId,
				NfType:       models.NFTYPE_UDM,
				NfStatus:     models.NFSTATUS_REGISTERED,
				PlmnList:     []models.PlmnId{{Mcc: "001", Mnc: "01"}},
				NfServices: []models.NFService{{
					ServiceName:      models.SERVICENAME_NUDM_SDM,
					NfServiceStatus:  models.NFSERVICESTATUS_REGISTERED,
					AllowedNfDomains: []string{pattern},
				}},
			}

			var nf models.NFProfile
			if err := NnrfNFManagementDataModel(&nf, nfprofile); err == nil {
				t.Fatalf("expected an error for allowedNfDomains pattern %q", pattern)
			}
		})
	}
}

// TestNnrfNFManagementDataModelAllowsSafeAllowedNfDomainsPatterns verifies
// that the ReDoS guard does not reject ordinary, safe allowedNfDomains
// patterns: single-level quantifiers, and alternation outside any quantifier.
func TestNnrfNFManagementDataModelAllowsSafeAllowedNfDomainsPatterns(t *testing.T) {
	originalNrfConfig := factory.NrfConfig
	t.Cleanup(func() { factory.NrfConfig = originalNrfConfig })
	factory.NrfConfig = factory.Config{Configuration: &factory.Configuration{}}

	safePatterns := []string{`^.*\.example\.com$`, `^(foo|bar)\.example\.com$`, `^amf[0-9]+\.example\.com$`, `^amfset-[0-9]?\.example\.com$`}
	for _, pattern := range safePatterns {
		t.Run(pattern, func(t *testing.T) {
			nfprofile := models.NFProfile{
				NfInstanceId: testNfInstanceId,
				NfType:       models.NFTYPE_UDM,
				NfStatus:     models.NFSTATUS_REGISTERED,
				PlmnList:     []models.PlmnId{{Mcc: "001", Mnc: "01"}},
				NfServices: []models.NFService{{
					ServiceName:      models.SERVICENAME_NUDM_SDM,
					NfServiceStatus:  models.NFSERVICESTATUS_REGISTERED,
					AllowedNfDomains: []string{pattern},
				}},
			}

			var nf models.NFProfile
			if err := NnrfNFManagementDataModel(&nf, nfprofile); err != nil {
				t.Fatalf("expected allowedNfDomains pattern %q to be accepted, got %v", pattern, err)
			}
		})
	}
}

// TestCollectServiceNames verifies that notification matching (used to find
// subscribers of NF status events) considers service names advertised via
// either the deprecated nfServices array or its TS 29.510 Rel-16 replacement,
// nfServiceList.
func TestCollectServiceNames(t *testing.T) {
	nfServiceList := map[string]models.NFService{
		testServiceInstanceId: {ServiceName: models.SERVICENAME_NUDM_SDM},
	}
	nfProfile := models.NFProfile{
		NfServices: []models.NFService{
			{ServiceName: models.SERVICENAME_NUDM_UEAU},
		},
		NfServiceList: &nfServiceList,
	}

	names := collectServiceNames(nfProfile)
	found := map[string]bool{}
	for _, n := range names {
		found[n.(string)] = true
	}
	if !found[string(models.SERVICENAME_NUDM_UEAU)] {
		t.Fatalf("expected service name from legacy nfServices to be collected, got %+v", names)
	}
	if !found[string(models.SERVICENAME_NUDM_SDM)] {
		t.Fatalf("expected service name from nfServiceList to be collected, got %+v", names)
	}
}
