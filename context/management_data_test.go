// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/openapi/v2/models"
)

// TestNnrfNFManagementDataModel_NfServiceList verifies that the nfServiceList
// field (the TS 29.510 Rel-16 replacement for the deprecated nfServices array)
// provided by a registering NF is preserved in the stored/returned profile.
func TestNnrfNFManagementDataModel_NfServiceList(t *testing.T) {
	factory.NrfConfig = factory.Config{Configuration: &factory.Configuration{}}

	nfServiceList := map[string]models.NFService{
		"svc-0": {
			ServiceInstanceId: "svc-0",
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
	svc, ok := (*got)["svc-0"]
	if !ok {
		t.Fatalf("expected svc-0 entry in nfServiceList, got %+v", *got)
	}
	if svc.GetServiceName() != models.SERVICENAME_NUDM_SDM {
		t.Fatalf("unexpected service name: %q", svc.GetServiceName())
	}
}
