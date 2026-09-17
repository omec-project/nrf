// Copyright (c) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"errors"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	testFieldLink            = "_link"
	testNfInstanceAmfLater   = "amf-later"
	testNfInstanceAmfEarlier = "amf-earlier"
	testFieldItem            = "item"
	testFieldHref            = "href"
	testNfInstanceUdm1       = "udm-1"
	testFieldNfStatus        = "nfstatus"
	testExampleFqdn          = "example.com"
	testServiceInstanceId    = "svc-0"
	testServiceNameNudmSdm   = "nudm-sdm"
)

type mockDiscoveryDBClient struct {
	dbadapter.DBInterface
}

type mockBSFDiscoveryDBClient struct {
	dbadapter.DBInterface
}

func (db *mockDiscoveryDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	switch collName {
	case collUriList:
		return map[string]interface{}{
			fieldNfType: nfTypeUDM,
			testFieldLink: map[string]interface{}{
				testFieldItem: []map[string]interface{}{{
					testFieldHref: "https://nrf:29510/nnrf-nfm/v1/nf-instances/udm-1",
				}},
			},
		}, nil
	case collNfProfile:
		if filter[fieldNfInstanceId] == testNfInstanceUdm1 {
			return map[string]interface{}{
				fieldNfInstanceId: testNfInstanceUdm1,
				fieldNfTypeLower:  nfTypeUDM,
				testFieldNfStatus: nfServiceStatusRegistered,
				fieldNfServices: []map[string]interface{}{{
					fieldServiceName:     "nudm-ueau",
					fieldNfServiceStatus: nfServiceStatusRegistered,
				}},
			}, nil
		}
	}

	return nil, nil
}

func (db *mockDiscoveryDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error) {
	if collName == collNfProfile {
		return []map[string]any{
			{
				fieldNfInstanceId: testNfInstanceUdm1,
				fieldNfTypeLower:  nfTypeUDM,
				testFieldNfStatus: nfServiceStatusRegistered,
				fieldNfServices: []map[string]any{
					{
						fieldServiceName:     "nudm-ueau",
						fieldNfServiceStatus: nfServiceStatusRegistered,
					},
				},
			},
		}, nil
	}
	return nil, nil
}

func (db *mockBSFDiscoveryDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error) {
	if collName != collNfProfile {
		return nil, nil
	}

	return []map[string]interface{}{{
		fieldNfInstanceId: "bsf-1",
		fieldNfTypeLower:  nfTypeBSF,
		testFieldNfStatus: nfServiceStatusRegistered,
	}}, nil
}

func TestBuildFilterAllowsUnsetAllowedNfTypes(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", nfTypeAUSF)
	query.Set("requester-nf-type", nfTypeAMF)

	filter := buildFilter(query)
	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
	}

	if len(andFilters) != 2 {
		t.Fatalf("expected 2 top-level filters, got %d", len(andFilters))
	}

	requesterFilter := andFilters[1]
	orFilters, ok := requesterFilter[mongoOpOr].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $or filter type: %T", requesterFilter[mongoOpOr])
	}

	if len(orFilters) != 2 {
		t.Fatalf("expected 2 requester alternatives, got %d", len(orFilters))
	}

	if got := orFilters[0]["allowednftypes"]; got != nfTypeAMF {
		t.Fatalf("expected requester filter to match AMF, got %#v", got)
	}

	if got, exists := orFilters[1]["allowednftypes"]; !exists || got != nil {
		t.Fatalf("expected second requester filter to allow null allowednftypes, got %#v", orFilters[1])
	}
}

// TestBuildFilterServiceNamesCoversNfServiceList verifies the Mongo filter for
// service-names discovery matches both the deprecated nfServices array and its
// TS 29.510 Rel-16 replacement, nfServiceList.
func TestBuildFilterServiceNamesCoversNfServiceList(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", nfTypeUDM)
	query.Set("requester-nf-type", nfTypeAMF)
	query.Set("service-names", testServiceNameNudmSdm)

	filter := buildFilter(query)
	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
	}

	var serviceNamesFilter bson.M
	for _, f := range andFilters {
		if _, exists := f[mongoOpOr]; exists {
			serviceNamesFilter = f
		}
	}
	if serviceNamesFilter == nil {
		t.Fatalf("expected an $or service-names filter among: %+v", andFilters)
	}

	orFilters, ok := serviceNamesFilter[mongoOpOr].([]bson.M)
	if !ok || len(orFilters) != 2 {
		t.Fatalf("expected 2 service-names alternatives, got %#v", serviceNamesFilter[mongoOpOr])
	}

	if _, exists := orFilters[0][fieldNfServices]; !exists {
		t.Fatalf("expected first alternative to match legacy nfservices field, got %#v", orFilters[0])
	}

	exprFilter, exists := orFilters[1][mongoOpExpr]
	if !exists {
		t.Fatalf("expected second alternative to be an $expr filter over nfServiceList, got %#v", orFilters[1])
	}
	if _, ok := exprFilter.(bson.M); !ok {
		t.Fatalf("expected $expr filter value to be bson.M, got %T", exprFilter)
	}
}

// TestBuildFilterRequesterNfInstanceFqdnCoversNfServiceList verifies the Mongo
// filter for the requester-nfinstance-fqdn (Query-4) discovery path matches
// both the deprecated nfServices array and its TS 29.510 Rel-16 replacement,
// nfServiceList, using $expr-based regex matching for both (allowedNfDomains
// holds ECMA-262 patterns per TS 29.510 clause 6.1.6.2.2).
func TestBuildFilterRequesterNfInstanceFqdnCoversNfServiceList(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", nfTypeSMF)
	query.Set("requester-nf-type", nfTypeAMF)
	query.Set(queryParamRequesterNfInstanceFqdn, testExampleFqdn)

	filter := buildFilter(query)
	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
	}

	var fqdnFilter bson.M
	for _, f := range andFilters {
		if orFilters, exists := f[mongoOpOr].([]bson.M); exists && len(orFilters) == 2 {
			if _, hasExpr := orFilters[0][mongoOpExpr]; hasExpr {
				fqdnFilter = f
			}
		}
	}
	if fqdnFilter == nil {
		t.Fatalf("expected a 2-alternative $or fqdn filter among: %+v", andFilters)
	}

	orFilters := fqdnFilter[mongoOpOr].([]bson.M)
	if _, exists := orFilters[0][mongoOpExpr]; !exists {
		t.Fatalf("expected first alternative to be an $expr filter over legacy nfservices, got %#v", orFilters[0])
	}
	if _, exists := orFilters[1][mongoOpExpr]; !exists {
		t.Fatalf("expected second alternative to be an $expr filter over nfServiceList, got %#v", orFilters[1])
	}
}

// TestBuildFilterRequesterNfInstanceFqdnIgnoresEmptyValue verifies that
// "?requester-nfinstance-fqdn=" (present but empty) adds no Mongo filter,
// mirroring the non-empty guard the URI-list/in-memory fallback path
// (matchesDiscoveryQuery) applies for the same parameter. Without this
// guard, the Mongo path would add a filter matching only services with no
// allowedNfDomains restriction, silently excluding restricted services that
// the fallback path returns unfiltered for the same empty value.
func TestBuildFilterRequesterNfInstanceFqdnIgnoresEmptyValue(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", nfTypeSMF)
	query.Set("requester-nf-type", nfTypeAMF)
	query.Set(queryParamRequesterNfInstanceFqdn, "")

	filter := buildFilter(query)
	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
	}

	for _, f := range andFilters {
		if orFilters, exists := f[mongoOpOr].([]bson.M); exists && len(orFilters) == 2 {
			if _, hasExpr := orFilters[0][mongoOpExpr]; hasExpr {
				t.Fatalf("expected no requester-nfinstance-fqdn filter for an empty value, got %#v", f)
			}
		}
	}
}

// TestBuildFilterSupportedFeaturesCoversNfServiceList verifies the Mongo
// filter for the supported-features (Query-34) discovery path matches both
// the deprecated nfServices array and its TS 29.510 Rel-16 replacement,
// nfServiceList.
func TestBuildFilterSupportedFeaturesCoversNfServiceList(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", nfTypeSMF)
	query.Set("requester-nf-type", nfTypeAMF)
	query.Set("supported-features", "1")

	filter := buildFilter(query)
	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
	}

	var supportedFeaturesFilter bson.M
	for _, f := range andFilters {
		if orFilters, exists := f[mongoOpOr].([]bson.M); exists && len(orFilters) == 2 {
			if _, hasExpr := orFilters[1][mongoOpExpr]; hasExpr {
				supportedFeaturesFilter = f
			}
		}
	}
	if supportedFeaturesFilter == nil {
		t.Fatalf("expected a 2-alternative $or supported-features filter among: %+v", andFilters)
	}

	orFilters := supportedFeaturesFilter[mongoOpOr].([]bson.M)
	if _, exists := orFilters[0][fieldNfServices]; !exists {
		t.Fatalf("expected first alternative to match legacy nfservices field, got %#v", orFilters[0])
	}
	if _, exists := orFilters[1][mongoOpExpr]; !exists {
		t.Fatalf("expected second alternative to be an $expr filter over nfServiceList, got %#v", orFilters[1])
	}
}

func TestFilterDiscoveryResultsAllowsUnsetAllowedNfTypes(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", nfTypeAUSF)
	query.Set("requester-nf-type", nfTypeAMF)
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

// TestFilterDiscoveryResultsMatchesNfServiceList verifies that service-names
// discovery matches profiles advertising services only via nfServiceList (the
// TS 29.510 Rel-16 replacement for the deprecated nfServices array)
func TestFilterDiscoveryResultsMatchesNfServiceList(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", nfTypeUDM)
	query.Set("service-names", testServiceNameNudmSdm)

	nfServiceList := map[string]models.NFService{
		testServiceInstanceId: {
			ServiceName:     models.SERVICENAME_NUDM_SDM,
			NfServiceStatus: models.NFSERVICESTATUS_REGISTERED,
		},
	}
	profiles := []models.NFProfileDiscovery{
		{
			NfInstanceId:  "udm-1",
			NfType:        models.NFTYPE_UDM,
			NfServiceList: &nfServiceList,
		},
	}

	filtered := filterDiscoveryResults(profiles, query)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 matching profile, got %d", len(filtered))
	}
	if filtered[0].NfInstanceId != "udm-1" {
		t.Fatalf("unexpected profile returned: %+v", filtered[0])
	}
}

// TestFilterDiscoveryResultsAppliesRequesterNfInstanceFqdn verifies that the
// URI-list fallback matcher (matchesDiscoveryQuery) applies the Query-4
// requester-nfinstance-fqdn predicate, matching services that allow the
// requester FQDN (or omit allowedNfDomains) via either nfServices or
// nfServiceList, while excluding services that restrict allowedNfDomains to
// other domains, including an explicitly empty list.
func TestFilterDiscoveryResultsAppliesRequesterNfInstanceFqdn(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", nfTypeUDM)
	query.Set(queryParamRequesterNfInstanceFqdn, testExampleFqdn)

	nfServiceList := map[string]models.NFService{
		testServiceInstanceId: {AllowedNfDomains: []string{testOtherFqdn}},
	}
	profiles := []models.NFProfileDiscovery{
		{
			NfInstanceId: "udm-allowed-legacy",
			NfType:       models.NFTYPE_UDM,
			NfServices:   []models.NFService{{AllowedNfDomains: []string{testExampleFqdn}}},
		},
		{
			NfInstanceId: "udm-no-restriction",
			NfType:       models.NFTYPE_UDM,
			NfServices:   []models.NFService{{}},
		},
		{
			NfInstanceId:  "udm-blocked",
			NfType:        models.NFTYPE_UDM,
			NfServiceList: &nfServiceList,
		},
		{
			NfInstanceId: "udm-blocked-empty-list",
			NfType:       models.NFTYPE_UDM,
			NfServices:   []models.NFService{{AllowedNfDomains: []string{}}},
		},
	}

	filtered := filterDiscoveryResults(profiles, query)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 matching profiles, got %d: %+v", len(filtered), filtered)
	}
	for _, p := range filtered {
		if p.NfInstanceId == "udm-blocked" || p.NfInstanceId == "udm-blocked-empty-list" {
			t.Fatalf("expected profile restricted to a different domain to be excluded, got %+v", filtered)
		}
	}
}

// TestMatchesAllowedNfDomainPatternRegex verifies that allowedNfDomains
// entries are evaluated as ECMA-262 regular expressions per TS 29.510 clause
// 6.1.6.2.2, not exact strings.
func TestMatchesAllowedNfDomainPatternRegex(t *testing.T) {
	if !matchesAllowedNfDomainPattern(`^.*\.example\.com$`, "amf1.example.com") {
		t.Fatal("expected wildcard subdomain pattern to match")
	}
	if matchesAllowedNfDomainPattern(`^.*\.example\.com$`, "example.com") {
		t.Fatal("expected wildcard subdomain pattern to require a subdomain")
	}
}

// TestMatchesAllowedNfDomainPatternInvalidRegexDoesNotMatch verifies that a
// malformed pattern is treated as non-matching instead of panicking or
// aborting discovery.
func TestMatchesAllowedNfDomainPatternInvalidRegexDoesNotMatch(t *testing.T) {
	if matchesAllowedNfDomainPattern("(unclosed", testExampleFqdn) {
		t.Fatal("expected invalid regex pattern to not match")
	}
}

// TestFilterDiscoveryResultsAppliesSupportedFeatures verifies that the
// URI-list fallback matcher (matchesDiscoveryQuery) applies the Query-34
// supported-features predicate against either nfServices or nfServiceList.
func TestFilterDiscoveryResultsAppliesSupportedFeatures(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", nfTypeUDM)
	query.Set("supported-features", "1")

	supportedFeatures := "1"
	nfServiceList := map[string]models.NFService{
		testServiceInstanceId: {SupportedFeatures: &supportedFeatures},
	}
	profiles := []models.NFProfileDiscovery{
		{
			NfInstanceId:  "udm-match",
			NfType:        models.NFTYPE_UDM,
			NfServiceList: &nfServiceList,
		},
		{
			NfInstanceId: "udm-no-match",
			NfType:       models.NFTYPE_UDM,
			NfServices:   []models.NFService{{}},
		},
	}

	filtered := filterDiscoveryResults(profiles, query)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 matching profile, got %d: %+v", len(filtered), filtered)
	}
	if filtered[0].NfInstanceId != "udm-match" {
		t.Fatalf("unexpected profile returned: %+v", filtered[0])
	}
}

func TestBuildFilterMatchesFullSmfDnn(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", nfTypeSMF)
	query.Set("requester-nf-type", nfTypeAMF)
	query.Set(queryParamDnn, "internet")

	filter := buildFilter(query)
	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
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
	orFilters, ok := dnnElemMatch[mongoOpOr].([]bson.M)
	if !ok {
		t.Fatalf("unexpected DNN matcher type: %T", dnnElemMatch[mongoOpOr])
	}
	if len(orFilters) != 2 {
		t.Fatalf("expected 2 DNN matcher alternatives, got %d", len(orFilters))
	}
	if got := orFilters[0][queryParamDnn]; got != "internet" {
		t.Fatalf("expected plain DNN match 'internet', got %#v", got)
	}
	if got := orFilters[1]["dnn.string"]; got != "internet" {
		t.Fatalf("expected object DNN match 'internet', got %#v", got)
	}
	if got := orFilters[0][queryParamDnn]; got == "i" {
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
	query.Set("target-nf-type", nfTypeUDM)
	query.Set("requester-nf-type", nfTypeAMF)

	profiles, err := loadDiscoveryProfilesFromURIList(query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].NfInstanceId != testNfInstanceUdm1 {
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
	query.Set("target-nf-type", nfTypeBSF)
	query.Set("requester-nf-type", nfTypeAMF)

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

// mockErroringDiscoveryDBClient simulates a MongoDB query failure on the
// primary NfProfile filter, e.g. one raised by an invalid $regexMatch pattern
// from a legacy or externally written profile, or an ordinary transient
// MongoDB/network error. RestfulAPIGetOne returns nil, nil (as it would for
// a missing document) so the URI-list fallback this error falls through to
// finds nothing, rather than panicking on the embedded nil DBInterface.
type mockErroringDiscoveryDBClient struct {
	dbadapter.DBInterface
}

func (db *mockErroringDiscoveryDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error) {
	if collName == collNfProfile {
		return nil, errors.New("simulated $regexMatch failure")
	}
	return nil, nil
}

func (db *mockErroringDiscoveryDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	return nil, nil
}

// TestNFDiscoveryProcedureFallsBackOnMongoQueryError verifies that a primary
// NfProfile query error (e.g. a legacy allowedNfDomains pattern breaking
// $regexMatch, or an ordinary transient MongoDB/network error) falls through
// to the URI-list/cache fallback instead of failing the whole request:
// treating every query error as a hard failure would make discovery
// unavailable even when the fallback could still serve results. With no
// URI-list data available either, the result is an empty but successful
// SearchResult, not a problem details failure.
func TestNFDiscoveryProcedureFallsBackOnMongoQueryError(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()
	dbadapter.DBClient = &mockErroringDiscoveryDBClient{}

	query := url.Values{}
	query.Set("target-nf-type", nfTypeUDM)
	query.Set("requester-nf-type", nfTypeAMF)

	response, problemDetails := NFDiscoveryProcedure(query)
	if problemDetails != nil {
		t.Fatalf("expected no problem details on a query error that falls back, got %+v", problemDetails)
	}
	if response == nil {
		t.Fatal("expected a SearchResult, not nil")
	}
	if len(response.NfInstances) != 0 {
		t.Fatalf("expected no NF instances, got %d", len(response.NfInstances))
	}
}

func TestNormalizeDiscoveryQueryParametersSupportsExplodedStructuredParams(t *testing.T) {
	query := url.Values{}
	openapi.ParameterAddToHeaderOrQuery(query, "target-plmn-list", []models.PlmnId{{Mcc: "001", Mnc: "01"}}, "", "")
	openapi.ParameterAddToHeaderOrQuery(query, fieldSnssais, []models.Snssai{{Sst: 1, Sd: openapi.PtrString("010203")}}, "", "")
	openapi.ParameterAddToHeaderOrQuery(query, "tai", models.Tai{PlmnId: models.PlmnId{Mcc: "001", Mnc: "01"}, Tac: "000001"}, "", "")
	openapi.ParameterAddToHeaderOrQuery(query, "guami", models.Guami{PlmnId: models.PlmnIdNid{Mcc: "001", Mnc: "01"}, AmfId: "000001"}, "", "")

	normalized := normalizeDiscoveryQueryParameters(query)

	if got := normalized.Get("target-plmn-list"); got == "" || got[0] != '{' {
		t.Fatalf("expected normalized target-plmn-list JSON, got %q", got)
	}
	if got := normalized.Get(fieldSnssais); got == "" || got[0] != '{' {
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
	query.Set("target-nf-type", nfTypeAMF)
	query.Set("requester-nf-type", nfTypeSMF)
	openapi.ParameterAddToHeaderOrQuery(query, "target-plmn-list", []models.PlmnId{{Mcc: "001", Mnc: "01"}}, "", "")
	openapi.ParameterAddToHeaderOrQuery(query, "tai", models.Tai{PlmnId: models.PlmnId{Mcc: "001", Mnc: "01"}, Tac: "000001"}, "", "")
	openapi.ParameterAddToHeaderOrQuery(query, "guami", models.Guami{PlmnId: models.PlmnIdNid{Mcc: "001", Mnc: "01"}, AmfId: "000001"}, "", "")

	filter := buildFilter(normalizeDiscoveryQueryParameters(query))
	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
	}

	var foundPLMN, foundTAI, foundGUAMI bool
	for _, candidate := range andFilters {
		if _, exists := candidate[mongoOpOr]; exists {
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
		queryParamTargetNfFqdn: {value: testExampleFqdn, negative: true},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
	}
	if len(andFilters) != 1 {
		t.Fatalf("expected 1 fqdn filter, got %d", len(andFilters))
	}

	fqdnFilter, ok := andFilters[0][fieldFqdn].(bson.M)
	if !ok {
		t.Fatalf("expected field-level fqdn filter, got %#v", andFilters[0])
	}
	if got := fqdnFilter[mongoOpNe]; got != testExampleFqdn {
		t.Fatalf("expected fqdn $ne match, got %#v", fqdnFilter)
	}
}

// TestComplexQueryFilterSubprocessMatchesNfServiceList verifies that
// complex-query service-names filtering covers nfServiceList (the TS 29.510
// Rel-16 replacement for the deprecated nfServices array), not just the
// legacy array, for both the positive and negated query semantics.
func TestComplexQueryFilterSubprocessMatchesNfServiceList(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		queryParamServiceNames: {value: testServiceNameNudmSdm},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok || len(andFilters) != 1 {
		t.Fatalf("expected 1 service-names filter, got %#v", filter[mongoOpAnd])
	}

	orFilters, ok := andFilters[0][mongoOpOr].([]bson.M)
	if !ok || len(orFilters) != 2 {
		t.Fatalf("expected 2 service-names alternatives, got %#v", andFilters[0])
	}
	if _, exists := orFilters[0][fieldNfServices]; !exists {
		t.Fatalf("expected first alternative to match legacy nfservices field, got %#v", orFilters[0])
	}
	if _, exists := orFilters[1][mongoOpExpr]; !exists {
		t.Fatalf("expected second alternative to be an $expr filter over nfServiceList, got %#v", orFilters[1])
	}
}

func TestComplexQueryFilterSubprocessNegatesServiceNamesWithNin(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		queryParamServiceNames: {value: testServiceNameNudmSdm, negative: true},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok || len(andFilters) != 1 {
		t.Fatalf("expected 1 service-names filter, got %#v", filter[mongoOpAnd])
	}

	orFilters, ok := andFilters[0][mongoOpOr].([]bson.M)
	if !ok || len(orFilters) != 2 {
		t.Fatalf("expected 2 service-names alternatives, got %#v", andFilters[0])
	}

	legacyFilter, ok := orFilters[0][fieldNfServices].(bson.M)
	if !ok {
		t.Fatalf("expected legacy nfservices filter, got %#v", orFilters[0])
	}
	elemMatch, ok := legacyFilter[mongoOpElemMatch].(bson.M)
	if !ok {
		t.Fatalf("expected $elemMatch document, got %#v", legacyFilter)
	}
	nameCond, ok := elemMatch[fieldServiceName].(bson.M)
	if !ok {
		t.Fatalf("expected servicename condition, got %#v", elemMatch)
	}
	if _, exists := nameCond["$nin"]; !exists {
		t.Fatalf("expected $nin negation on legacy nfservices filter, got %#v", nameCond)
	}

	if _, exists := orFilters[1][mongoOpExpr]; !exists {
		t.Fatalf("expected second alternative to be an $expr filter over nfServiceList, got %#v", orFilters[1])
	}
}

// TestComplexQueryFilterSubprocessMatchesRequesterNfInstanceFqdnNfServiceList
// verifies that complex-query requester-nfinstance-fqdn (Query-4) filtering
// covers nfServiceList in addition to the legacy nfServices array.
func TestComplexQueryFilterSubprocessMatchesRequesterNfInstanceFqdnNfServiceList(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		queryParamRequesterNfInstanceFqdn: {value: testExampleFqdn},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok || len(andFilters) != 1 {
		t.Fatalf("expected 1 fqdn filter, got %#v", filter[mongoOpAnd])
	}

	orFilters, ok := andFilters[0][mongoOpOr].([]bson.M)
	if !ok || len(orFilters) != 2 {
		t.Fatalf("expected 2 fqdn alternatives, got %#v", andFilters[0])
	}
	if _, exists := orFilters[0][mongoOpExpr]; !exists {
		t.Fatalf("expected first alternative to be an $expr filter over legacy nfservices, got %#v", orFilters[0])
	}
	if _, exists := orFilters[1][mongoOpExpr]; !exists {
		t.Fatalf("expected second alternative to be an $expr filter over nfServiceList, got %#v", orFilters[1])
	}
}

// TestComplexQueryFilterSubprocessIgnoresEmptyRequesterNfInstanceFqdn verifies
// that an atom with an empty requester-nfinstance-fqdn value adds no Mongo
// filter, mirroring TestBuildFilterRequesterNfInstanceFqdnIgnoresEmptyValue
// for the simple-query path. Without this guard, the empty value would still
// reach $regexMatch as an empty regex, which matches every stored domain and
// authorizes restricted services, unlike the simple-query and in-memory
// fallback paths, which treat an empty parameter as absent.
func TestComplexQueryFilterSubprocessIgnoresEmptyRequesterNfInstanceFqdn(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		queryParamRequesterNfInstanceFqdn: {value: ""},
	}, COMPLEX_QUERY_TYPE_DNF)

	if andFilters, ok := filter[mongoOpAnd].([]bson.M); ok && len(andFilters) != 0 {
		t.Fatalf("expected no requester-nfinstance-fqdn filter for an empty value, got %#v", andFilters)
	}
}

// extractAnyMatchCond drills into the bson.M produced by nfServicesAnyMatch or
// nfServiceListAnyMatch to return the "cond" passed to $filter, so tests can
// assert on the exact predicate used to match nfServices/nfServiceList entries.
func extractAnyMatchCond(t *testing.T, exprFilter bson.M) bson.M {
	t.Helper()
	gt, ok := exprFilter[mongoOpExpr].(bson.M)["$gt"].([]any)
	if !ok || len(gt) != 2 {
		t.Fatalf("unexpected $expr structure: %#v", exprFilter)
	}
	size, ok := gt[0].(bson.M)["$size"].(bson.M)
	if !ok {
		t.Fatalf("unexpected $size structure: %#v", gt[0])
	}
	filterDoc, ok := size["$filter"].(bson.M)
	if !ok {
		t.Fatalf("unexpected $filter structure: %#v", size)
	}
	cond, ok := filterDoc["cond"].(bson.M)
	if !ok {
		t.Fatalf("unexpected cond structure: %#v", filterDoc)
	}
	return cond
}

// TestComplexQueryFilterSubprocessRequesterNfInstanceFqdnAllowsMissingOrMatchingPattern
// verifies that the positive (non-negated) requester-nfinstance-fqdn query
// matches nfServices/nfServiceList entries whose allowedNfDomains contains a
// pattern matching the FQDN (an ECMA-262 regex per TS 29.510 clause
// 6.1.6.2.2), or omits allowedNfDomains entirely (unrestricted), identically
// for both representations.
func TestComplexQueryFilterSubprocessRequesterNfInstanceFqdnAllowsMissingOrMatchingPattern(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		queryParamRequesterNfInstanceFqdn: {value: testExampleFqdn},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters := filter[mongoOpAnd].([]bson.M)
	orFilters := andFilters[0][mongoOpOr].([]bson.M)

	for _, cond := range []bson.M{extractAnyMatchCond(t, orFilters[0]), extractAnyMatchCond(t, orFilters[1])} {
		svcOr, exists := cond[mongoOpOr].([]bson.M)
		if !exists || len(svcOr) != 2 {
			t.Fatalf("expected cond to allow missing-or-matching-pattern domain, got %#v", cond)
		}
		if _, hasEq := svcOr[0][mongoOpEq]; !hasEq {
			t.Fatalf("expected first alternative to check for a missing allowedNfDomains, got %#v", svcOr[0])
		}
		anyElementTrue, exists := svcOr[1]["$anyElementTrue"].(bson.A)
		if !exists || len(anyElementTrue) != 1 {
			t.Fatalf("expected second alternative to be $anyElementTrue over a one-element array wrapping a $map, got %#v", svcOr[1])
		}
		mapContainer, exists := anyElementTrue[0].(bson.M)
		if !exists {
			t.Fatalf("expected $anyElementTrue operand to be a document, got %#v", anyElementTrue[0])
		}
		mapExpr, exists := mapContainer["$map"].(bson.M)
		if !exists {
			t.Fatalf("expected $anyElementTrue to wrap a $map, got %#v", mapContainer)
		}
		inExpr, exists := mapExpr["in"].(bson.M)
		if !exists {
			t.Fatalf("expected $map to have an 'in' expression, got %#v", mapExpr)
		}
		regexMatch, exists := inExpr["$regexMatch"].(bson.M)
		if !exists {
			t.Fatalf("expected 'in' expression to be $regexMatch, got %#v", inExpr)
		}
		if got := regexMatch["input"]; !reflect.DeepEqual(got, bson.M{"$literal": testExampleFqdn}) {
			t.Fatalf("expected $regexMatch input to be $literal-wrapped %q, got %#v", testExampleFqdn, got)
		}
		if got := regexMatch["regex"]; got != "$$domain" {
			t.Fatalf("expected $regexMatch regex to reference the stored pattern, got %#v", got)
		}
	}
}

// TestComplexQueryFilterSubprocessNegatesRequesterNfInstanceFqdnWithNor verifies
// that the negated requester-nfinstance-fqdn query uses $nor (not the invalid
// top-level $not) to match profiles where no service allows the requester
// FQDN.
func TestComplexQueryFilterSubprocessNegatesRequesterNfInstanceFqdnWithNor(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		queryParamRequesterNfInstanceFqdn: {value: testExampleFqdn, negative: true},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters := filter[mongoOpAnd].([]bson.M)
	norFilters, ok := andFilters[0][mongoOpNor].([]bson.M)
	if !ok || len(norFilters) != 1 {
		t.Fatalf("expected $nor-wrapped fqdn filter, got %#v", andFilters[0])
	}
	orFilters, ok := norFilters[0][mongoOpOr].([]bson.M)
	if !ok || len(orFilters) != 2 {
		t.Fatalf("expected 2 fqdn alternatives inside $nor, got %#v", norFilters[0])
	}
	if _, exists := orFilters[0][mongoOpExpr]; !exists {
		t.Fatalf("expected first alternative to be an $expr filter over legacy nfservices, got %#v", orFilters[0])
	}
	if _, exists := orFilters[1][mongoOpExpr]; !exists {
		t.Fatalf("expected second alternative to be an $expr filter over nfServiceList, got %#v", orFilters[1])
	}
}

// TestComplexQueryFilterSubprocessMatchesSupportedFeaturesNfServiceList
// verifies that complex-query supported-features (Query-34) filtering covers
// nfServiceList in addition to the legacy nfServices array.
func TestComplexQueryFilterSubprocessMatchesSupportedFeaturesNfServiceList(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		queryParamSupportedFeatures: {value: "1"},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok || len(andFilters) != 1 {
		t.Fatalf("expected 1 supported-features filter, got %#v", filter[mongoOpAnd])
	}

	orFilters, ok := andFilters[0][mongoOpOr].([]bson.M)
	if !ok || len(orFilters) != 2 {
		t.Fatalf("expected 2 supported-features alternatives, got %#v", andFilters[0])
	}
	if _, exists := orFilters[0][fieldNfServices]; !exists {
		t.Fatalf("expected first alternative to match legacy nfservices field, got %#v", orFilters[0])
	}
	if _, exists := orFilters[1][mongoOpExpr]; !exists {
		t.Fatalf("expected second alternative to be an $expr filter over nfServiceList, got %#v", orFilters[1])
	}
}

// TestComplexQueryFilterSubprocessNegatesSupportedFeaturesWithNor verifies that
// the negated supported-features complex query uses $nor (not the invalid
// top-level $not) to negate the whole legacy-or-nfServiceList alternative.
func TestComplexQueryFilterSubprocessNegatesSupportedFeaturesWithNor(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		queryParamSupportedFeatures: {value: "1", negative: true},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok || len(andFilters) != 1 {
		t.Fatalf("expected 1 supported-features filter, got %#v", filter[mongoOpAnd])
	}

	norFilters, ok := andFilters[0][mongoOpNor].([]bson.M)
	if !ok || len(norFilters) != 1 {
		t.Fatalf("expected $nor-wrapped supported-features filter, got %#v", andFilters[0])
	}
	orFilters, ok := norFilters[0][mongoOpOr].([]bson.M)
	if !ok || len(orFilters) != 2 {
		t.Fatalf("expected 2 supported-features alternatives inside $nor, got %#v", norFilters[0])
	}
	if _, exists := orFilters[0][fieldNfServices]; !exists {
		t.Fatalf("expected first alternative to match legacy nfservices field, got %#v", orFilters[0])
	}
	if _, exists := orFilters[1][mongoOpExpr]; !exists {
		t.Fatalf("expected second alternative to be an $expr filter over nfServiceList, got %#v", orFilters[1])
	}
}

func TestComplexQueryFilterSubprocessBuildsSnssaisElemMatchDocument(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		fieldSnssais: {value: `{"sst":1,"sd":"010203"}`},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
	}
	if len(andFilters) != 1 {
		t.Fatalf("expected 1 snssais filter, got %d", len(andFilters))
	}

	snssaisFilter, ok := andFilters[0][fieldSnssais].(bson.M)
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
	query.Set("target-nf-type", nfTypeAMF)
	query.Set("requester-nf-type", nfTypeSMF)
	query.Set(fieldSnssais, `{"sst":1}`)

	filter := buildFilter(query)
	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
	}

	var snssaisFilter bson.M
	for _, candidate := range andFilters {
		if orFilters, exists := candidate[mongoOpOr].(bson.A); exists {
			for _, orFilter := range orFilters {
				orFilterMap, mapOK := orFilter.(bson.M)
				if !mapOK {
					continue
				}
				if _, exists := orFilterMap[fieldSnssais]; exists {
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

	fieldFilter, ok := snssaisFilter[fieldSnssais].(bson.M)
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
	query.Set("target-nf-type", nfTypeAMF)
	query.Set("requester-nf-type", nfTypeSMF)
	query.Set(fieldSnssais, `{"sst":1`)

	filter := buildFilter(query)
	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
	}

	for _, candidate := range andFilters {
		orFilters, exists := candidate[mongoOpOr].(bson.A)
		if !exists {
			continue
		}
		for _, orFilter := range orFilters {
			orFilterMap, mapOK := orFilter.(bson.M)
			if !mapOK {
				continue
			}
			if _, hasSnssais := orFilterMap[fieldSnssais]; hasSnssais {
				t.Fatalf("expected invalid snssais value to be skipped, got %#v", candidate)
			}
		}
	}
}

func TestComplexQueryFilterSubprocessBuildsSnssaisElemMatchWithoutSd(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		fieldSnssais: {value: `{"sst":1}`},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
	}
	if len(andFilters) != 1 {
		t.Fatalf("expected 1 snssais filter, got %d", len(andFilters))
	}

	snssaisFilter, ok := andFilters[0][fieldSnssais].(bson.M)
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
		fieldSnssais: {value: `{"sst":1,"sd":"010203"}`, negative: true},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
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

	snssaisFilter, ok := norFilters[0][fieldSnssais].(bson.M)
	if !ok {
		t.Fatalf("expected nested snssais field filter, got %#v", norFilters[0])
	}
	if _, ok := snssaisFilter[mongoOpElemMatch].(bson.M); !ok {
		t.Fatalf("expected nested snssais $elemMatch document, got %#v", snssaisFilter[mongoOpElemMatch])
	}
}

func TestComplexQueryFilterSubprocessSkipsInvalidSnssaisValue(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		fieldSnssais: {value: `{"sst":1`},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok {
		t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
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

	firstFilter, ok := filters[0][fieldSnssais].(bson.M)
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

	secondFilter, ok := filters[1][fieldSnssais].(bson.M)
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
	if collName == collNfProfile {
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
			fieldNfInstanceId: testNfInstanceAmfLater,
			fieldNfTypeLower:  nfTypeAMF,
			testFieldNfStatus: nfServiceStatusRegistered,
			fieldExpireAt:     later,
		},
		{
			fieldNfInstanceId: testNfInstanceAmfEarlier,
			fieldNfTypeLower:  nfTypeAMF,
			testFieldNfStatus: nfServiceStatusRegistered,
			fieldExpireAt:     earlier,
		},
	}

	originalDBClient := dbadapter.DBClient
	dbadapter.DBClient = &mockSortingDBClient{profiles: profiles}
	defer func() { dbadapter.DBClient = originalDBClient }()

	query := url.Values{}
	query.Set("target-nf-type", nfTypeAMF)
	query.Set("requester-nf-type", nfTypeSMF)

	response, problemDetails := NFDiscoveryProcedure(query)
	if problemDetails != nil {
		t.Fatalf("unexpected problem details: %+v", problemDetails)
	}
	if response == nil || len(response.NfInstances) != 2 {
		t.Fatalf("expected 2 NF instances, got %+v", response)
	}
	if got := response.NfInstances[0].NfInstanceId; got != testNfInstanceAmfEarlier {
		t.Errorf("expected amf-earlier first (earlier expiry), got %q", got)
	}
	if got := response.NfInstances[1].NfInstanceId; got != testNfInstanceAmfLater {
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
			fieldNfInstanceId: testNfInstanceAmfLater,
			fieldNfTypeLower:  nfTypeAMF,
			testFieldNfStatus: nfServiceStatusRegistered,
			fieldExpireAt:     laterbsonDT,
		},
		{
			fieldNfInstanceId: "amf-no-expiry",
			fieldNfTypeLower:  nfTypeAMF,
			testFieldNfStatus: nfServiceStatusRegistered,
			// no expireAt field
		},
		{
			fieldNfInstanceId: testNfInstanceAmfEarlier,
			fieldNfTypeLower:  nfTypeAMF,
			testFieldNfStatus: nfServiceStatusRegistered,
			fieldExpireAt:     earliertimeTime,
		},
	}

	originalDBClient := dbadapter.DBClient
	dbadapter.DBClient = &mockSortingDBClient{profiles: profiles}
	defer func() { dbadapter.DBClient = originalDBClient }()

	query := url.Values{}
	query.Set("target-nf-type", nfTypeAMF)
	query.Set("requester-nf-type", nfTypeSMF)

	response, problemDetails := NFDiscoveryProcedure(query)
	if problemDetails != nil {
		t.Fatalf("unexpected problem details: %+v", problemDetails)
	}
	if response == nil || len(response.NfInstances) != 3 {
		t.Fatalf("expected 3 NF instances, got %+v", response)
	}
	if got := response.NfInstances[0].NfInstanceId; got != testNfInstanceAmfEarlier {
		t.Errorf("expected amf-earlier first (earliest expiry), got %q", got)
	}
	if got := response.NfInstances[1].NfInstanceId; got != testNfInstanceAmfLater {
		t.Errorf("expected amf-later second (later expiry), got %q", got)
	}
	if got := response.NfInstances[2].NfInstanceId; got != "amf-no-expiry" {
		t.Errorf("expected amf-no-expiry last (missing expireAt), got %q", got)
	}
}

func TestProfileCacheGetReturnsHitForValidEntry(t *testing.T) {
	c := &nfProfileCache{entries: make(map[string]profileCacheEntry)}
	p := models.NFProfileDiscovery{NfInstanceId: "hit-test", NfType: models.NFTYPE_AMF}
	c.set(p, time.Now().Add(60*time.Second))

	got, ok := c.get("hit-test")
	if !ok {
		t.Fatal("expected cache hit for valid entry")
	}
	if got.NfInstanceId != "hit-test" {
		t.Fatalf("unexpected profile: %+v", got)
	}
}

func TestProfileCacheGetReturnsMissForExpiredEntry(t *testing.T) {
	c := &nfProfileCache{entries: make(map[string]profileCacheEntry)}
	c.entries["expired"] = profileCacheEntry{
		profile:   models.NFProfileDiscovery{NfInstanceId: "expired"},
		expiresAt: time.Now().Add(-time.Second),
	}

	if _, ok := c.get("expired"); ok {
		t.Fatal("expected cache miss for expired entry")
	}
}

func TestProfileCacheGetDeletesExpiredEntryOnRead(t *testing.T) {
	c := &nfProfileCache{entries: make(map[string]profileCacheEntry)}
	c.entries["stale"] = profileCacheEntry{
		profile:   models.NFProfileDiscovery{NfInstanceId: "stale"},
		expiresAt: time.Now().Add(-time.Second),
	}

	c.get("stale") // should evict

	c.mu.RLock()
	_, stillPresent := c.entries["stale"]
	c.mu.RUnlock()
	if stillPresent {
		t.Fatal("expected expired entry to be deleted from cache on read")
	}
}

// mockCacheTestDBClient returns no NfProfile results to verify the cache path.
type mockCacheTestDBClient struct {
	dbadapter.DBInterface
	getManyCalled bool
}

func (db *mockCacheTestDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	if collName == collUriList {
		return map[string]interface{}{
			fieldNfType: nfTypeAMF,
			testFieldLink: map[string]interface{}{
				testFieldItem: []map[string]interface{}{{
					testFieldHref: "https://nrf:29510/nnrf-nfm/v1/nf-instances/amf-cached",
				}},
			},
		}, nil
	}
	return nil, nil
}

func (db *mockCacheTestDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error) {
	db.getManyCalled = true
	return nil, nil
}

func TestLoadDiscoveryProfilesFromURIListServesFromCache(t *testing.T) {
	const testID = "amf-cached"
	profileCache.evict(testID)
	defer profileCache.evict(testID)

	p := models.NFProfileDiscovery{
		NfInstanceId: testID,
		NfType:       models.NFTYPE_AMF,
		NfStatus:     models.NFSTATUS_REGISTERED,
	}
	profileCache.set(p, time.Now().Add(60*time.Second))

	mockDB := &mockCacheTestDBClient{}
	originalDBClient := dbadapter.DBClient
	dbadapter.DBClient = mockDB
	defer func() { dbadapter.DBClient = originalDBClient }()

	query := url.Values{}
	query.Set("target-nf-type", nfTypeAMF)
	query.Set("requester-nf-type", nfTypeSMF)

	profiles, err := loadDiscoveryProfilesFromURIList(query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 cached profile, got %d", len(profiles))
	}
	if profiles[0].NfInstanceId != testID {
		t.Fatalf("unexpected profile id: %s", profiles[0].NfInstanceId)
	}
	if mockDB.getManyCalled {
		t.Fatal("expected DB not to be queried when all profiles are cached")
	}
}

// mockMalformedBatchDBClient returns one valid and one constraint-violating profile.
type mockMalformedBatchDBClient struct {
	dbadapter.DBInterface
}

func (db *mockMalformedBatchDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	if collName == collUriList {
		return map[string]interface{}{
			fieldNfType: nfTypeAMF,
			testFieldLink: map[string]interface{}{
				testFieldItem: []map[string]interface{}{
					{testFieldHref: "https://nrf:29510/nnrf-nfm/v1/nf-instances/amf-valid"},
					{testFieldHref: "https://nrf:29510/nnrf-nfm/v1/nf-instances/amf-invalid"},
				},
			},
		}, nil
	}
	return nil, nil
}

func (db *mockMalformedBatchDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error) {
	return []map[string]any{
		{
			fieldNfInstanceId: "amf-valid",
			fieldNfTypeLower:  nfTypeAMF,
			testFieldNfStatus: nfServiceStatusRegistered,
		},
		{
			fieldNfInstanceId: "amf-invalid",
			fieldNfTypeLower:  nfTypeAMF,
			testFieldNfStatus: nfServiceStatusRegistered,
			"priority":        999999, // violates TS 29.510 constraint [0, 65535]
		},
	}, nil
}

func TestLoadDiscoveryProfilesFromURIListBatchDecodeErrorFallsBackToPerProfile(t *testing.T) {
	profileCache.evict("amf-valid")
	profileCache.evict("amf-invalid")
	defer func() {
		profileCache.evict("amf-valid")
		profileCache.evict("amf-invalid")
	}()

	originalDBClient := dbadapter.DBClient
	dbadapter.DBClient = &mockMalformedBatchDBClient{}
	defer func() { dbadapter.DBClient = originalDBClient }()

	query := url.Values{}
	query.Set("target-nf-type", nfTypeAMF)
	query.Set("requester-nf-type", nfTypeSMF)

	profiles, err := loadDiscoveryProfilesFromURIList(query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 valid profile (invalid entry dropped), got %d", len(profiles))
	}
	if profiles[0].NfInstanceId != "amf-valid" {
		t.Fatalf("unexpected profile id: %s", profiles[0].NfInstanceId)
	}
}
