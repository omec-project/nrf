// Copyright (c) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"net/url"
	"testing"

	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"go.mongodb.org/mongo-driver/bson"
)

type mockDiscoveryDBClient struct {
	dbadapter.DBInterface
}

type mockBSFDiscoveryDBClient struct {
	dbadapter.DBInterface
}

func (db *mockDiscoveryDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	switch collName {
	case "urilist":
		return map[string]interface{}{
			"nfType": "UDM",
			"_link": map[string]interface{}{
				"item": []map[string]interface{}{{
					"href": "https://nrf:29510/nnrf-nfm/v1/nf-instances/udm-1",
				}},
			},
		}, nil
	case "NfProfile":
		if filter["nfinstanceid"] == "udm-1" {
			return map[string]interface{}{
				"nfinstanceid": "udm-1",
				"nftype":       "UDM",
				"nfstatus":     "REGISTERED",
				"nfservices": []map[string]interface{}{{
					"servicename":     "nudm-ueau",
					"nfservicestatus": "REGISTERED",
				}},
			}, nil
		}
	}

	return nil, nil
}

func (db *mockDiscoveryDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error) {
	if collName == "NfProfile" {
		return []map[string]any{
			{
				"nfinstanceid": "udm-1",
				"nftype":       "UDM",
				"nfstatus":     "REGISTERED",
				"nfservices": []map[string]any{
					{
						"servicename":     "nudm-ueau",
						"nfservicestatus": "REGISTERED",
					},
				},
			},
		}, nil
	}
	return nil, nil
}

func (db *mockBSFDiscoveryDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error) {
	if collName != "NfProfile" {
		return nil, nil
	}

	return []map[string]interface{}{{
		"nfinstanceid": "bsf-1",
		"nftype":       "BSF",
		"nfstatus":     "REGISTERED",
	}}, nil
}

func TestBuildFilterAllowsUnsetAllowedNfTypes(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", "AUSF")
	query.Set("requester-nf-type", "AMF")

	filter := buildFilter(query)
	andFilters, ok := filter["$and"].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter["$and"])
	}

	if len(andFilters) != 2 {
		t.Fatalf("expected 2 top-level filters, got %d", len(andFilters))
	}

	requesterFilter := andFilters[1]
	orFilters, ok := requesterFilter["$or"].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $or filter type: %T", requesterFilter["$or"])
	}

	if len(orFilters) != 2 {
		t.Fatalf("expected 2 requester alternatives, got %d", len(orFilters))
	}

	if got := orFilters[0]["allowednftypes"]; got != "AMF" {
		t.Fatalf("expected requester filter to match AMF, got %#v", got)
	}

	if got, exists := orFilters[1]["allowednftypes"]; !exists || got != nil {
		t.Fatalf("expected second requester filter to allow null allowednftypes, got %#v", orFilters[1])
	}
}

func TestFilterDiscoveryResultsAllowsUnsetAllowedNfTypes(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", "AUSF")
	query.Set("requester-nf-type", "AMF")
	query.Set("service-names", "nausf-auth")

	profiles := []models.NFProfileDiscovery{
		{
			NfInstanceId: "ausf-1",
			NfType:       models.NFTYPE_AUSF,
			NfServices: []models.NFService{
				{
					ServiceName:     models.SERVICENAME_NAUSF_AUTH,
					NfServiceStatus: models.NFSERVICESTATUS_REGISTERED,
				},
			},
		},
	}

	filtered := filterDiscoveryResults(profiles, query)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 matching profile, got %d", len(filtered))
	}
	if filtered[0].NfInstanceId != "ausf-1" {
		t.Fatalf("unexpected profile returned: %+v", filtered[0])
	}
}

func TestBuildFilterMatchesFullSmfDnn(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", "SMF")
	query.Set("requester-nf-type", "AMF")
	query.Set("dnn", "internet")

	filter := buildFilter(query)
	andFilters, ok := filter["$and"].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter["$and"])
	}

	var dnnFilter bson.M
	for _, candidate := range andFilters {
		if _, exists := candidate["smfinfo.snssaismfinfolist"]; exists {
			dnnFilter = candidate
			break
		}
	}
	if dnnFilter == nil {
		t.Fatalf("expected SMF DNN filter in %+v", andFilters)
	}

	snssaiInfoFilter, ok := dnnFilter["smfinfo.snssaismfinfolist"].(bson.M)
	if !ok {
		t.Fatalf("unexpected snssai info filter type: %T", dnnFilter["smfinfo.snssaismfinfolist"])
	}
	elemMatch, ok := snssaiInfoFilter["$elemMatch"].(bson.M)
	if !ok {
		t.Fatalf("unexpected $elemMatch type: %T", snssaiInfoFilter["$elemMatch"])
	}
	dnnListFilter, ok := elemMatch["dnnsmfinfolist"].(bson.M)
	if !ok {
		t.Fatalf("unexpected dnn list filter type: %T", elemMatch["dnnsmfinfolist"])
	}
	dnnElemMatch, ok := dnnListFilter["$elemMatch"].(bson.M)
	if !ok {
		t.Fatalf("unexpected DNN $elemMatch type: %T", dnnListFilter["$elemMatch"])
	}
	orFilters, ok := dnnElemMatch["$or"].([]bson.M)
	if !ok {
		t.Fatalf("unexpected DNN matcher type: %T", dnnElemMatch["$or"])
	}
	if len(orFilters) != 2 {
		t.Fatalf("expected 2 DNN matcher alternatives, got %d", len(orFilters))
	}
	if got := orFilters[0]["dnn"]; got != "internet" {
		t.Fatalf("expected plain DNN match 'internet', got %#v", got)
	}
	if got := orFilters[1]["dnn.string"]; got != "internet" {
		t.Fatalf("expected object DNN match 'internet', got %#v", got)
	}
	if got := orFilters[0]["dnn"]; got == "i" {
		t.Fatalf("unexpected first-character DNN match: %#v", got)
	}
}

func TestLoadDiscoveryProfilesFromURIList(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	dbadapter.DBClient = &mockDiscoveryDBClient{}
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()

	query := url.Values{}
	query.Set("target-nf-type", "UDM")
	query.Set("requester-nf-type", "AMF")

	profiles, err := loadDiscoveryProfilesFromURIList(query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].NfInstanceId != "udm-1" {
		t.Fatalf("unexpected profile id: %+v", profiles[0])
	}
	if profiles[0].NfType != models.NFTYPE_UDM {
		t.Fatalf("unexpected profile type: %s", profiles[0].NfType)
	}
}

func TestNFDiscoveryProcedureHandlesBSFProfileWithoutBsfInfo(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()

	query := url.Values{}
	query.Set("target-nf-type", "BSF")
	query.Set("requester-nf-type", "AMF")

	dbadapter.DBClient = &mockBSFDiscoveryDBClient{}

	response, problemDetails := NFDiscoveryProcedure(query)
	if problemDetails != nil {
		t.Fatalf("unexpected problem details: %+v", problemDetails)
	}
	if response == nil {
		t.Fatal("expected discovery response")
	}
	if len(response.NfInstances) != 1 {
		t.Fatalf("expected one BSF instance, got %d", len(response.NfInstances))
	}
	if response.NfInstances[0].BsfInfo != nil {
		t.Fatalf("expected nil BsfInfo, got %+v", response.NfInstances[0].BsfInfo)
	}
}

func TestNormalizeDiscoveryQueryParametersSupportsExplodedStructuredParams(t *testing.T) {
	query := url.Values{}
	openapi.ParameterAddToHeaderOrQuery(query, "target-plmn-list", []models.PlmnId{{Mcc: "001", Mnc: "01"}}, "", "")
	openapi.ParameterAddToHeaderOrQuery(query, "snssais", []models.Snssai{{Sst: 1, Sd: openapi.PtrString("010203")}}, "", "")
	openapi.ParameterAddToHeaderOrQuery(query, "tai", models.Tai{PlmnId: models.PlmnId{Mcc: "001", Mnc: "01"}, Tac: "000001"}, "", "")
	openapi.ParameterAddToHeaderOrQuery(query, "guami", models.Guami{PlmnId: models.PlmnIdNid{Mcc: "001", Mnc: "01"}, AmfId: "000001"}, "", "")

	normalized := normalizeDiscoveryQueryParameters(query)

	if got := normalized.Get("target-plmn-list"); got == "" || got[0] != '{' {
		t.Fatalf("expected normalized target-plmn-list JSON, got %q", got)
	}
	if got := normalized.Get("snssais"); got == "" || got[0] != '{' {
		t.Fatalf("expected normalized snssais JSON, got %q", got)
	}
	if got := normalized.Get("tai"); got == "" || got[0] != '{' {
		t.Fatalf("expected normalized tai JSON, got %q", got)
	}
	if got := normalized.Get("guami"); got == "" || got[0] != '{' {
		t.Fatalf("expected normalized guami JSON, got %q", got)
	}
}

func TestBuildFilterSupportsExplodedStructuredParams(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", "AMF")
	query.Set("requester-nf-type", "SMF")
	openapi.ParameterAddToHeaderOrQuery(query, "target-plmn-list", []models.PlmnId{{Mcc: "001", Mnc: "01"}}, "", "")
	openapi.ParameterAddToHeaderOrQuery(query, "tai", models.Tai{PlmnId: models.PlmnId{Mcc: "001", Mnc: "01"}, Tac: "000001"}, "", "")
	openapi.ParameterAddToHeaderOrQuery(query, "guami", models.Guami{PlmnId: models.PlmnIdNid{Mcc: "001", Mnc: "01"}, AmfId: "000001"}, "", "")

	filter := buildFilter(normalizeDiscoveryQueryParameters(query))
	andFilters, ok := filter["$and"].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter["$and"])
	}

	var foundPLMN, foundTAI, foundGUAMI bool
	for _, candidate := range andFilters {
		if _, exists := candidate["$or"]; exists {
			foundPLMN = true
		}
		if _, exists := candidate["amfinfo.tailist"]; exists {
			foundTAI = true
		}
		if _, exists := candidate["amfinfo.guamilist"]; exists {
			foundGUAMI = true
		}
	}

	if !foundPLMN || !foundTAI || !foundGUAMI {
		t.Fatalf("expected PLMN, TAI and GUAMI filters, got %+v", andFilters)
	}
}
