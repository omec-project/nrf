// Copyright (c) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"net/url"
	"testing"
	"time"

	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"go.mongodb.org/mongo-driver/v2/bson"
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

func TestComplexQueryFilterSubprocessNegatesTargetNfFqdnWithNe(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		queryParamTargetNfFqdn: {value: "example.com", negative: true},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter["$and"].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter["$and"])
	}
	if len(andFilters) != 1 {
		t.Fatalf("expected 1 fqdn filter, got %d", len(andFilters))
	}

	fqdnFilter, ok := andFilters[0]["fqdn"].(bson.M)
	if !ok {
		t.Fatalf("expected field-level fqdn filter, got %#v", andFilters[0])
	}
	if got := fqdnFilter["$ne"]; got != "example.com" {
		t.Fatalf("expected fqdn $ne match, got %#v", fqdnFilter)
	}
}

func TestComplexQueryFilterSubprocessBuildsSnssaisElemMatchDocument(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		"snssais": {value: `{"sst":1,"sd":"010203"}`},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter["$and"].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter["$and"])
	}
	if len(andFilters) != 1 {
		t.Fatalf("expected 1 snssais filter, got %d", len(andFilters))
	}

	snssaisFilter, ok := andFilters[0]["snssais"].(bson.M)
	if !ok {
		t.Fatalf("expected snssais field filter, got %#v", andFilters[0])
	}
	elemMatch, ok := snssaisFilter[mongoOpElemMatch].(bson.M)
	if !ok {
		t.Fatalf("expected snssais $elemMatch document, got %#v", snssaisFilter[mongoOpElemMatch])
	}
	if got := elemMatch["sst"]; got != int32(1) {
		t.Fatalf("expected snssais sst 1, got %#v", elemMatch)
	}
	if got := elemMatch["sd"]; got != "010203" {
		t.Fatalf("expected snssais sd 010203, got %#v", elemMatch)
	}
}

func TestBuildFilterSupportsSnssaiWithoutSd(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", "AMF")
	query.Set("requester-nf-type", "SMF")
	query.Set("snssais", `{"sst":1}`)

	filter := buildFilter(query)
	andFilters, ok := filter["$and"].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter["$and"])
	}

	var snssaisFilter bson.M
	for _, candidate := range andFilters {
		if orFilters, exists := candidate["$or"].(bson.A); exists {
			for _, orFilter := range orFilters {
				orFilterMap, mapOK := orFilter.(bson.M)
				if !mapOK {
					continue
				}
				if _, exists := orFilterMap["snssais"]; exists {
					snssaisFilter = orFilterMap
					break
				}
			}
		}
		if snssaisFilter != nil {
			break
		}
	}
	if snssaisFilter == nil {
		t.Fatalf("expected snssais filter in %+v", andFilters)
	}

	fieldFilter, ok := snssaisFilter["snssais"].(bson.M)
	if !ok {
		t.Fatalf("expected snssais field filter, got %#v", snssaisFilter)
	}
	elemMatch, ok := fieldFilter[mongoOpElemMatch].(bson.M)
	if !ok {
		t.Fatalf("expected snssais $elemMatch document, got %#v", fieldFilter[mongoOpElemMatch])
	}
	if got := elemMatch["sst"]; got != int32(1) {
		t.Fatalf("expected snssais sst 1, got %#v", elemMatch)
	}
	if _, exists := elemMatch["sd"]; exists {
		t.Fatalf("did not expect sd in %#v", elemMatch)
	}
}

func TestBuildFilterSkipsInvalidSnssaisValue(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", "AMF")
	query.Set("requester-nf-type", "SMF")
	query.Set("snssais", `{"sst":1`)

	filter := buildFilter(query)
	andFilters, ok := filter["$and"].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter["$and"])
	}

	for _, candidate := range andFilters {
		orFilters, exists := candidate["$or"].(bson.A)
		if !exists {
			continue
		}
		for _, orFilter := range orFilters {
			orFilterMap, mapOK := orFilter.(bson.M)
			if !mapOK {
				continue
			}
			if _, hasSnssais := orFilterMap["snssais"]; hasSnssais {
				t.Fatalf("expected invalid snssais value to be skipped, got %#v", candidate)
			}
		}
	}
}

func TestComplexQueryFilterSubprocessBuildsSnssaisElemMatchWithoutSd(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		"snssais": {value: `{"sst":1}`},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter["$and"].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter["$and"])
	}
	if len(andFilters) != 1 {
		t.Fatalf("expected 1 snssais filter, got %d", len(andFilters))
	}

	snssaisFilter, ok := andFilters[0]["snssais"].(bson.M)
	if !ok {
		t.Fatalf("expected snssais field filter, got %#v", andFilters[0])
	}
	elemMatch, ok := snssaisFilter[mongoOpElemMatch].(bson.M)
	if !ok {
		t.Fatalf("expected snssais $elemMatch document, got %#v", snssaisFilter[mongoOpElemMatch])
	}
	if got := elemMatch["sst"]; got != int32(1) {
		t.Fatalf("expected snssais sst 1, got %#v", elemMatch)
	}
	if _, exists := elemMatch["sd"]; exists {
		t.Fatalf("did not expect sd in %#v", elemMatch)
	}
}

func TestComplexQueryFilterSubprocessNegatesSnssaisWithNor(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		"snssais": {value: `{"sst":1,"sd":"010203"}`, negative: true},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter["$and"].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter["$and"])
	}
	if len(andFilters) != 1 {
		t.Fatalf("expected 1 snssais filter, got %d", len(andFilters))
	}

	norFilters, ok := andFilters[0]["$nor"].([]bson.M)
	if !ok {
		t.Fatalf("expected $nor snssais negation, got %#v", andFilters[0])
	}
	if len(norFilters) != 1 {
		t.Fatalf("expected 1 negated snssais clause, got %d", len(norFilters))
	}

	snssaisFilter, ok := norFilters[0]["snssais"].(bson.M)
	if !ok {
		t.Fatalf("expected nested snssais field filter, got %#v", norFilters[0])
	}
	if _, ok := snssaisFilter[mongoOpElemMatch].(bson.M); !ok {
		t.Fatalf("expected nested snssais $elemMatch document, got %#v", snssaisFilter[mongoOpElemMatch])
	}
}

func TestComplexQueryFilterSubprocessSkipsInvalidSnssaisValue(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		"snssais": {value: `{"sst":1`},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter["$and"].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter["$and"])
	}
	if len(andFilters) != 0 {
		t.Fatalf("expected invalid snssais value to be skipped, got %#v", andFilters)
	}
}

func TestBuildSnssaisElemMatchFiltersHandlesMultipleCommaSeparatedObjects(t *testing.T) {
	filters := buildSnssaisElemMatchFilters(`{"sst":1,"sd":"010203"},{"sst":1,"sd":"040506"}`)
	if len(filters) != 2 {
		t.Fatalf("expected 2 snssais filters, got %d: %#v", len(filters), filters)
	}

	firstFilter, ok := filters[0]["snssais"].(bson.M)
	if !ok {
		t.Fatalf("expected first snssais field filter, got %#v", filters[0])
	}
	firstElemMatch, ok := firstFilter[mongoOpElemMatch].(bson.M)
	if !ok {
		t.Fatalf("expected first snssais $elemMatch document, got %#v", firstFilter[mongoOpElemMatch])
	}
	if got := firstElemMatch["sd"]; got != "010203" {
		t.Fatalf("expected first snssais sd 010203, got %#v", firstElemMatch)
	}

	secondFilter, ok := filters[1]["snssais"].(bson.M)
	if !ok {
		t.Fatalf("expected second snssais field filter, got %#v", filters[1])
	}
	secondElemMatch, ok := secondFilter[mongoOpElemMatch].(bson.M)
	if !ok {
		t.Fatalf("expected second snssais $elemMatch document, got %#v", secondFilter[mongoOpElemMatch])
	}
	if got := secondElemMatch["sd"]; got != "040506" {
		t.Fatalf("expected second snssais sd 040506, got %#v", secondElemMatch)
	}
}

// mockSortingDBClient returns a configurable slice of raw NF profiles for
// testing sort behaviour in NFDiscoveryProcedure.
type mockSortingDBClient struct {
	dbadapter.DBInterface
	profiles []map[string]any
}

func (db *mockSortingDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]any, error) {
	if collName == "NfProfile" {
		return db.profiles, nil
	}
	return nil, nil
}

func TestRawExpireAtToTimeWithBsonDateTime(t *testing.T) {
	expected := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dt := bson.DateTime(expected.UnixMilli())
	got, ok := rawExpireAtToTime(dt)
	if !ok {
		t.Fatal("expected ok=true for bson.DateTime")
	}
	if !got.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}

func TestRawExpireAtToTimeWithTimeTime(t *testing.T) {
	expected := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got, ok := rawExpireAtToTime(expected)
	if !ok {
		t.Fatal("expected ok=true for time.Time")
	}
	if !got.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}

func TestRawExpireAtToTimeWithUnsupportedType(t *testing.T) {
	if _, ok := rawExpireAtToTime("2026-01-01T00:00:00Z"); ok {
		t.Fatal("expected ok=false for string value")
	}
	if _, ok := rawExpireAtToTime(nil); ok {
		t.Fatal("expected ok=false for nil")
	}
	if _, ok := rawExpireAtToTime(int64(1234567890)); ok {
		t.Fatal("expected ok=false for int64")
	}
}

func TestNFDiscoveryProcedureSortsProfilesByExpireAt(t *testing.T) {
	now := time.Now()
	earlier := bson.DateTime(now.Add(-1 * time.Hour).UnixMilli())
	later := bson.DateTime(now.Add(1 * time.Hour).UnixMilli())

	// Profiles deliberately in reverse order: later expiry first, earlier second.
	profiles := []map[string]any{
		{
			"nfinstanceid": "amf-later",
			"nftype":       "AMF",
			"nfstatus":     "REGISTERED",
			"expireAt":     later,
		},
		{
			"nfinstanceid": "amf-earlier",
			"nftype":       "AMF",
			"nfstatus":     "REGISTERED",
			"expireAt":     earlier,
		},
	}

	originalDBClient := dbadapter.DBClient
	dbadapter.DBClient = &mockSortingDBClient{profiles: profiles}
	defer func() { dbadapter.DBClient = originalDBClient }()

	query := url.Values{}
	query.Set("target-nf-type", "AMF")
	query.Set("requester-nf-type", "SMF")

	response, problemDetails := NFDiscoveryProcedure(query)
	if problemDetails != nil {
		t.Fatalf("unexpected problem details: %+v", problemDetails)
	}
	if response == nil || len(response.NfInstances) != 2 {
		t.Fatalf("expected 2 NF instances, got %+v", response)
	}
	if got := response.NfInstances[0].NfInstanceId; got != "amf-earlier" {
		t.Errorf("expected amf-earlier first (earlier expiry), got %q", got)
	}
	if got := response.NfInstances[1].NfInstanceId; got != "amf-later" {
		t.Errorf("expected amf-later second (later expiry), got %q", got)
	}
}

func TestNFDiscoveryProcedureSortsMixedExpireAtTypesAndMissing(t *testing.T) {
	now := time.Now()
	// earlier uses time.Time to exercise that branch of rawExpireAtToTime
	earliertimeTime := now.Add(-1 * time.Hour)
	// later uses bson.DateTime (the normal MongoDB decode type)
	laterbsonDT := bson.DateTime(now.Add(1 * time.Hour).UnixMilli())

	// Profiles in scrambled order: later, no-expiry, earlier.
	profiles := []map[string]any{
		{
			"nfinstanceid": "amf-later",
			"nftype":       "AMF",
			"nfstatus":     "REGISTERED",
			"expireAt":     laterbsonDT,
		},
		{
			"nfinstanceid": "amf-no-expiry",
			"nftype":       "AMF",
			"nfstatus":     "REGISTERED",
			// no expireAt field
		},
		{
			"nfinstanceid": "amf-earlier",
			"nftype":       "AMF",
			"nfstatus":     "REGISTERED",
			"expireAt":     earliertimeTime,
		},
	}

	originalDBClient := dbadapter.DBClient
	dbadapter.DBClient = &mockSortingDBClient{profiles: profiles}
	defer func() { dbadapter.DBClient = originalDBClient }()

	query := url.Values{}
	query.Set("target-nf-type", "AMF")
	query.Set("requester-nf-type", "SMF")

	response, problemDetails := NFDiscoveryProcedure(query)
	if problemDetails != nil {
		t.Fatalf("unexpected problem details: %+v", problemDetails)
	}
	if response == nil || len(response.NfInstances) != 3 {
		t.Fatalf("expected 3 NF instances, got %+v", response)
	}
	if got := response.NfInstances[0].NfInstanceId; got != "amf-earlier" {
		t.Errorf("expected amf-earlier first (earliest expiry), got %q", got)
	}
	if got := response.NfInstances[1].NfInstanceId; got != "amf-later" {
		t.Errorf("expected amf-later second (later expiry), got %q", got)
	}
	if got := response.NfInstances[2].NfInstanceId; got != "amf-no-expiry" {
		t.Errorf("expected amf-no-expiry last (missing expireAt), got %q", got)
	}
}
