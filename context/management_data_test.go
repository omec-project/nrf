// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/openapi/v2/models"
)

const testServiceInstanceId = "svc-0"

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
		NfInstanceId:  "test-instance-id",
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
