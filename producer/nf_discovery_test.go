// Copyright (c) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	testFieldLink                    = "_link"
	testNfInstanceAmfLater           = "amf-later"
	testNfInstanceAmfEarlier         = "amf-earlier"
	testFieldItem                    = "item"
	testFieldHref                    = "href"
	testNfInstanceUdm1               = "udm-1"
	testNfInstanceUdmAllowed         = "udm-allowed"
	testNfInstanceUdmBlocked         = "udm-blocked"
	testNfInstanceUdmFallbackAllowed = "udm-fallback-allowed"
	testFieldNfStatus                = "nfstatus"
	testExampleFqdn                  = "example.com"
	testServiceInstanceId            = "svc-0"
	testServiceNameNudmSdm           = "nudm-sdm"
	testNfInstanceAusf1              = "ausf-1"
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

// TestBuildFilterOmitsRequesterNfInstanceFqdn verifies that the simple-query
// Mongo filter never includes a requester-nf-instance-fqdn (Query-4)
// predicate, regardless of whether the parameter is present: that predicate
// is evaluated in Go instead (see filterByRequesterNfInstanceFqdn), so that a
// stored allowedNfDomains pattern is never executed by MongoDB's PCRE-based
// $regexMatch, which (unlike Go's RE2) can be driven into catastrophic
// backtracking by a crafted pattern.
func TestBuildFilterOmitsRequesterNfInstanceFqdn(t *testing.T) {
	for _, fqdnValue := range []string{testExampleFqdn, ""} {
		query := url.Values{}
		query.Set("target-nf-type", nfTypeSMF)
		query.Set("requester-nf-type", nfTypeAMF)
		query.Set(queryParamRequesterNfInstanceFqdn, fqdnValue)

		filter := buildFilter(query)
		andFilters, ok := filter[mongoOpAnd].([]bson.M)
		if !ok {
			t.Fatalf("unexpected $and filter type: %T", filter[mongoOpAnd])
		}
		for _, f := range andFilters {
			if orFilters, exists := f[mongoOpOr].([]bson.M); exists && len(orFilters) == 2 {
				if _, hasExpr := orFilters[0][mongoOpExpr]; hasExpr {
					t.Fatalf("expected no requester-nf-instance-fqdn filter in the Mongo query, got %#v", f)
				}
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
			NfInstanceId: testNfInstanceAusf1,
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
	if filtered[0].NfInstanceId != testNfInstanceAusf1 {
		t.Fatalf("unexpected profile returned: %+v", filtered[0])
	}
}

// TestFilterDiscoveryResultsRejectsPresentEmptyAllowedNfTypes verifies that a
// profile with an explicitly present but empty allowedNfTypes list is
// excluded by the requester-nf-type fallback filter, mirroring the Mongo
// predicate (handleRequesterNfType): {"allowednftypes": nil} only matches a
// missing/null field, not a present empty array, so treating "present but
// empty" as unrestricted here would let the fallback return profiles the
// primary Mongo query would have excluded.
func TestFilterDiscoveryResultsRejectsPresentEmptyAllowedNfTypes(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", nfTypeAUSF)
	query.Set("requester-nf-type", nfTypeAMF)

	profiles := []models.NFProfileDiscovery{
		{
			NfInstanceId:   testNfInstanceAusf1,
			NfType:         models.NFTYPE_AUSF,
			AllowedNfTypes: []models.NFType{},
		},
	}

	filtered := filterDiscoveryResults(profiles, query)
	if len(filtered) != 0 {
		t.Fatalf("expected profile with present-but-empty allowedNfTypes to be excluded, got %+v", filtered)
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
			NfInstanceId:  testNfInstanceUdm1,
			NfType:        models.NFTYPE_UDM,
			NfServiceList: &nfServiceList,
		},
	}

	filtered := filterDiscoveryResults(profiles, query)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 matching profile, got %d", len(filtered))
	}
	if filtered[0].NfInstanceId != testNfInstanceUdm1 {
		t.Fatalf("unexpected profile returned: %+v", filtered[0])
	}
}

// TestFilterByRequesterNfInstanceFqdnAppliesPredicate verifies that
// filterByRequesterNfInstanceFqdn (the single place, shared by the primary
// MongoDB-backed query and the URI-list fallback, that evaluates the
// requester-nf-instance-fqdn predicate) matches profiles with a service that
// allows the requester FQDN (or omits allowedNfDomains) via either
// nfServices or nfServiceList, while excluding profiles restricted to other
// domains, including via an explicitly empty list.
func TestFilterByRequesterNfInstanceFqdnAppliesPredicate(t *testing.T) {
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
			NfInstanceId:  testNfInstanceUdmBlocked,
			NfType:        models.NFTYPE_UDM,
			NfServiceList: &nfServiceList,
		},
		{
			NfInstanceId: "udm-blocked-empty-list",
			NfType:       models.NFTYPE_UDM,
			NfServices:   []models.NFService{{AllowedNfDomains: []string{}}},
		},
	}

	filtered := filterByRequesterNfInstanceFqdn(profiles, query)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 matching profiles, got %d: %+v", len(filtered), filtered)
	}
	for _, p := range filtered {
		if p.NfInstanceId == testNfInstanceUdmBlocked || p.NfInstanceId == "udm-blocked-empty-list" {
			t.Fatalf("expected profile restricted to a different domain to be excluded, got %+v", filtered)
		}
	}
}

// TestFilterByRequesterNfInstanceFqdnIgnoresEmptyValue verifies that an
// absent or empty requester-nf-instance-fqdn parameter returns profiles
// unchanged, rather than excluding every profile.
func TestFilterByRequesterNfInstanceFqdnIgnoresEmptyValue(t *testing.T) {
	profiles := []models.NFProfileDiscovery{
		{NfInstanceId: testNfInstanceUdm1, NfType: models.NFTYPE_UDM},
	}

	for _, query := range []url.Values{{}, {queryParamRequesterNfInstanceFqdn: {""}}} {
		filtered := filterByRequesterNfInstanceFqdn(profiles, query)
		if len(filtered) != 1 {
			t.Fatalf("expected profiles to pass through unfiltered for query %+v, got %+v", query, filtered)
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

// TestCompileAllowedNfDomainPatternCacheIsBounded verifies that
// allowedNfDomainPatternCache does not grow without bound: since
// ValidateAllowedNfDomains only bounds a single pattern's length/complexity,
// not how many distinct patterns are ever seen, an unbounded cache would grow
// forever as NFs register or patch with different patterns over time.
func TestCompileAllowedNfDomainPatternCacheIsBounded(t *testing.T) {
	allowedNfDomainPatternCacheMu.Lock()
	originalCache := allowedNfDomainPatternCache
	allowedNfDomainPatternCache = make(map[string]compiledAllowedNfDomainPattern)
	allowedNfDomainPatternCacheMu.Unlock()
	t.Cleanup(func() {
		allowedNfDomainPatternCacheMu.Lock()
		allowedNfDomainPatternCache = originalCache
		allowedNfDomainPatternCacheMu.Unlock()
	})

	for i := range maxAllowedNfDomainPatternCacheEntries + 1 {
		if _, err := compileAllowedNfDomainPattern(fmt.Sprintf("^host%d\\.example\\.com$", i)); err != nil {
			t.Fatalf("unexpected compile error: %v", err)
		}
	}

	allowedNfDomainPatternCacheMu.Lock()
	size := len(allowedNfDomainPatternCache)
	allowedNfDomainPatternCacheMu.Unlock()
	if size > maxAllowedNfDomainPatternCacheEntries {
		t.Fatalf("expected cache size to stay at or below %d, got %d", maxAllowedNfDomainPatternCacheEntries, size)
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

// mockFqdnDiscoveryDBClient returns two profiles matching every Mongo-side
// discovery filter (target-nf-type); only requester-nf-instance-fqdn, applied
// in Go after the query, should tell them apart.
type mockFqdnDiscoveryDBClient struct {
	dbadapter.DBInterface
}

func (db *mockFqdnDiscoveryDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error) {
	if collName != collNfProfile {
		return nil, nil
	}
	return []map[string]interface{}{
		{
			fieldNfInstanceId: testNfInstanceUdmAllowed,
			fieldNfTypeLower:  nfTypeUDM,
			testFieldNfStatus: nfServiceStatusRegistered,
			fieldNfServices: []map[string]interface{}{{
				fieldServiceName:     testServiceNameNudmSdm,
				fieldNfServiceStatus: nfServiceStatusRegistered,
			}},
		},
		{
			fieldNfInstanceId: testNfInstanceUdmBlocked,
			fieldNfTypeLower:  nfTypeUDM,
			testFieldNfStatus: nfServiceStatusRegistered,
			fieldNfServices: []map[string]interface{}{{
				fieldServiceName:      testServiceNameNudmSdm,
				fieldNfServiceStatus:  nfServiceStatusRegistered,
				fieldAllowedNfDomains: []string{testOtherFqdn},
			}},
		},
	}, nil
}

// TestNFDiscoveryProcedureAppliesRequesterNfInstanceFqdnToPrimaryQueryResults
// verifies that requester-nf-instance-fqdn still narrows the primary
// MongoDB-backed query's results end to end, even though buildFilter no
// longer sends that predicate to MongoDB (see filterByRequesterNfInstanceFqdn).
func TestNFDiscoveryProcedureAppliesRequesterNfInstanceFqdnToPrimaryQueryResults(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()
	dbadapter.DBClient = &mockFqdnDiscoveryDBClient{}

	query := url.Values{}
	query.Set("target-nf-type", nfTypeUDM)
	query.Set("requester-nf-type", nfTypeAMF)
	query.Set(queryParamRequesterNfInstanceFqdn, testExampleFqdn)

	response, problemDetails := NFDiscoveryProcedure(query)
	if problemDetails != nil {
		t.Fatalf("unexpected problem details: %+v", problemDetails)
	}
	if response == nil {
		t.Fatal("expected discovery response")
	}
	if len(response.NfInstances) != 1 {
		t.Fatalf("expected 1 NF instance after requester-nf-instance-fqdn filtering, got %d: %+v", len(response.NfInstances), response.NfInstances)
	}
	if response.NfInstances[0].NfInstanceId != testNfInstanceUdmAllowed {
		t.Fatalf("expected the unrestricted profile to be returned, got %+v", response.NfInstances[0])
	}
}

// mockFqdnFallbackDiscoveryDBClient returns, from the primary MongoDB query,
// a single profile that requester-nf-instance-fqdn filtering rejects, and a
// URI-list pointing at a separate, unrestricted profile served from the
// profile cache.
type mockFqdnFallbackDiscoveryDBClient struct {
	dbadapter.DBInterface
}

func (db *mockFqdnFallbackDiscoveryDBClient) RestfulAPIGetOne(collName string, filter bson.M) (map[string]interface{}, error) {
	if collName == collUriList {
		return map[string]interface{}{
			fieldNfType: nfTypeUDM,
			testFieldLink: map[string]interface{}{
				testFieldItem: []map[string]interface{}{{
					testFieldHref: "https://nrf:29510/nnrf-nfm/v1/nf-instances/" + testNfInstanceUdmFallbackAllowed,
				}},
			},
		}, nil
	}
	return nil, nil
}

func (db *mockFqdnFallbackDiscoveryDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error) {
	if collName != collNfProfile {
		return nil, nil
	}
	return []map[string]interface{}{
		{
			fieldNfInstanceId: testNfInstanceUdmBlocked,
			fieldNfTypeLower:  nfTypeUDM,
			testFieldNfStatus: nfServiceStatusRegistered,
			fieldNfServices: []map[string]interface{}{{
				fieldServiceName:      testServiceNameNudmSdm,
				fieldNfServiceStatus:  nfServiceStatusRegistered,
				fieldAllowedNfDomains: []string{testOtherFqdn},
			}},
		},
	}, nil
}

// TestNFDiscoveryProcedureFallsBackWhenRequesterNfInstanceFqdnRejectsAllPrimaryResults
// verifies that a non-empty primary MongoDB result whose only profile is
// rejected by requester-nf-instance-fqdn filtering still triggers the
// URI-list fallback, instead of returning an empty response: the fallback
// decision inside sortNFProfiles must be made after applying that predicate,
// not before.
func TestNFDiscoveryProcedureFallsBackWhenRequesterNfInstanceFqdnRejectsAllPrimaryResults(t *testing.T) {
	profileCache.evict(testNfInstanceUdmFallbackAllowed)
	defer profileCache.evict(testNfInstanceUdmFallbackAllowed)
	profileCache.set(models.NFProfileDiscovery{
		NfInstanceId: testNfInstanceUdmFallbackAllowed,
		NfType:       models.NFTYPE_UDM,
		NfStatus:     models.NFSTATUS_REGISTERED,
		NfServices:   []models.NFService{{}},
	}, time.Now().Add(60*time.Second))

	originalDBClient := dbadapter.DBClient
	defer func() { dbadapter.DBClient = originalDBClient }()
	dbadapter.DBClient = &mockFqdnFallbackDiscoveryDBClient{}

	query := url.Values{}
	query.Set("target-nf-type", nfTypeUDM)
	query.Set("requester-nf-type", nfTypeAMF)
	query.Set(queryParamRequesterNfInstanceFqdn, testExampleFqdn)

	response, problemDetails := NFDiscoveryProcedure(query)
	if problemDetails != nil {
		t.Fatalf("unexpected problem details: %+v", problemDetails)
	}
	if response == nil {
		t.Fatal("expected discovery response")
	}
	if len(response.NfInstances) != 1 {
		t.Fatalf("expected the URI-list fallback to run and return 1 NF instance, got %d: %+v", len(response.NfInstances), response.NfInstances)
	}
	if response.NfInstances[0].NfInstanceId != testNfInstanceUdmFallbackAllowed {
		t.Fatalf("expected the fallback profile %q, got %+v", testNfInstanceUdmFallbackAllowed, response.NfInstances[0])
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

// TestNFDiscoveryProcedureFailsClosedOnMongoQueryErrorWithUnsupportedFallbackParam
// verifies that a primary NfProfile query error is reported as a failure,
// rather than falling back, when the request uses a query parameter (here,
// target-plmn-list) the URI-list fallback does not evaluate: falling back
// regardless would silently return profiles the full MongoDB query would
// have excluded.
func TestNFDiscoveryProcedureFailsClosedOnMongoQueryErrorWithUnsupportedFallbackParam(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()
	dbadapter.DBClient = &mockErroringDiscoveryDBClient{}

	query := url.Values{}
	query.Set("target-nf-type", nfTypeUDM)
	query.Set("requester-nf-type", nfTypeAMF)
	query.Set("target-plmn-list", `{"mcc":"001","mnc":"01"}`)

	response, problemDetails := NFDiscoveryProcedure(query)
	if response != nil {
		t.Fatalf("expected no SearchResult when failing closed, got %+v", response)
	}
	if problemDetails == nil {
		t.Fatal("expected problem details when the query cannot be safely served by the fallback")
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

// TestComplexQueryFilterSubprocessMatchesAllForCnfRequesterNfInstanceFqdnUnit
// verifies that a CNF unit (whose atoms are OR'd) containing a
// requester-nf-instance-fqdn atom never sends that atom's allowedNfDomains
// pattern to MongoDB's $regexMatch: the whole clause is matched
// unconditionally instead - a safe superset, since dropping only the FQDN
// disjunct while keeping a sibling atom's condition would make the OR
// narrower than the real clause - and filterByComplexQuery restores
// exactness in Go afterwards.
func TestComplexQueryFilterSubprocessMatchesAllForCnfRequesterNfInstanceFqdnUnit(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		queryParamRequesterNfInstanceFqdn: {value: testExampleFqdn},
		queryParamTargetNFType:            {value: nfTypeUDM},
	}, COMPLEX_QUERY_TYPE_CNF)

	orFilters, ok := filter[mongoOpOr].([]bson.M)
	if !ok || len(orFilters) != 1 || len(orFilters[0]) != 0 {
		t.Fatalf("expected a single match-all alternative, got %#v", filter[mongoOpOr])
	}
}

// TestComplexQueryFilterSubprocessDropsFqdnFromDnfUnit verifies that a DNF
// unit (whose atoms are AND'd) containing a requester-nf-instance-fqdn atom
// keeps any sibling atom's Mongo condition - unlike a CNF unit, dropping just
// the FQDN atom here is a safe superset, since removing one conjunct from an
// AND only broadens the match - and never sends the FQDN atom itself to
// MongoDB.
func TestComplexQueryFilterSubprocessDropsFqdnFromDnfUnit(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		queryParamRequesterNfInstanceFqdn: {value: testExampleFqdn},
		queryParamTargetNFType:            {value: nfTypeUDM},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok || len(andFilters) != 1 {
		t.Fatalf("expected 1 target-nf-type filter, got %#v", filter[mongoOpAnd])
	}
	if got := andFilters[0][fieldNfTypeLower]; got != nfTypeUDM {
		t.Fatalf("expected the surviving alternative to match target-nf-type, got %#v", andFilters[0])
	}
}

// TestComplexQueryFilterSubprocessMatchesAllForFqdnOnlyUnit verifies that a
// unit consisting solely of a requester-nf-instance-fqdn atom - in either its
// CNF or DNF form - matches unconditionally in Mongo, rather than leaving an
// empty $or/$and array (which MongoDB rejects).
func TestComplexQueryFilterSubprocessMatchesAllForFqdnOnlyUnit(t *testing.T) {
	tests := map[string]struct {
		complexQueryType string
		operator         string
	}{
		"CNF": {COMPLEX_QUERY_TYPE_CNF, mongoOpOr},
		"DNF": {COMPLEX_QUERY_TYPE_DNF, mongoOpAnd},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			filter := complexQueryFilterSubprocess(map[string]*AtomElem{
				queryParamRequesterNfInstanceFqdn: {value: testExampleFqdn},
			}, tc.complexQueryType)

			operatorFilters, ok := filter[tc.operator].([]bson.M)
			if !ok || len(operatorFilters) != 1 || len(operatorFilters[0]) != 0 {
				t.Fatalf("expected a single match-all alternative, got %#v", filter[tc.operator])
			}
		})
	}
}

// TestComplexQueryFilterSubprocessIgnoresEmptyRequesterNfInstanceFqdn verifies
// that an atom with an empty requester-nf-instance-fqdn value adds no Mongo
// condition for the atom itself, mirroring TestBuildFilterOmitsRequesterNfInstanceFqdn
// for the simple-query path, and (since it is then the only atom in the
// unit) still matches unconditionally rather than leaving an empty array.
func TestComplexQueryFilterSubprocessIgnoresEmptyRequesterNfInstanceFqdn(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		queryParamRequesterNfInstanceFqdn: {value: ""},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok || len(andFilters) != 1 || len(andFilters[0]) != 0 {
		t.Fatalf("expected a single match-all alternative for an empty value, got %#v", filter[mongoOpAnd])
	}
}

// TestValidateComplexQueryAllowsRequesterNfInstanceFqdnAtom verifies that a
// complexQuery request is allowed, in both its CNF and DNF forms, when it
// contains only a requester-nf-instance-fqdn atom: that atom is fully
// evaluated in Go (RE2) afterwards - see filterByComplexQuery and
// matchesComplexQuery - so it no longer needs to be rejected the way it did
// when the only alternative was MongoDB's PCRE-based $regexMatch.
func TestValidateComplexQueryAllowsRequesterNfInstanceFqdnAtom(t *testing.T) {
	tests := map[string]string{
		"CNF": `{"cnfUnits":[{"cnfUnit":[{"attr":"requester-nf-instance-fqdn","value":"example.com"}]}]}`,
		"DNF": `{"dnfUnits":[{"dnfUnit":[{"attr":"requester-nf-instance-fqdn","value":"example.com"}]}]}`,
	}
	for name, complexQuery := range tests {
		t.Run(name, func(t *testing.T) {
			query := url.Values{}
			query.Set("complexQuery", complexQuery)

			if problemDetails := validateComplexQuery(query); problemDetails != nil {
				t.Fatalf("unexpected problem details: %+v", problemDetails)
			}
		})
	}
}

// TestValidateComplexQueryRejectsRequesterNfInstanceFqdnMixedWithUnsupportedAttr
// verifies that a complexQuery request combining a requester-nf-instance-fqdn
// atom with an attribute matchesComplexQueryAtom does not support (e.g. tai)
// is still rejected: filterByComplexQuery could not exactly re-evaluate such
// a request in Go, so accepting it would either require sending the FQDN
// atom's allowedNfDomains pattern to MongoDB's $regexMatch (the catastrophic
// backtracking risk this predicate exists to avoid) or silently
// mis-evaluating the unsupported atom.
func TestValidateComplexQueryRejectsRequesterNfInstanceFqdnMixedWithUnsupportedAttr(t *testing.T) {
	query := url.Values{}
	query.Set("complexQuery", `{"cnfUnits":[{"cnfUnit":[{"attr":"requester-nf-instance-fqdn","value":"example.com"},{"attr":"tai","value":"{}"}]}]}`)

	problemDetails := validateComplexQuery(query)
	if problemDetails == nil {
		t.Fatal("expected problem details rejecting requester-nf-instance-fqdn combined with an unsupported attribute")
	}
}

// TestValidateComplexQueryAllowsOtherAtoms verifies that validateComplexQuery
// does not reject complexQuery atoms unrelated to requester-nf-instance-fqdn.
func TestValidateComplexQueryAllowsOtherAtoms(t *testing.T) {
	query := url.Values{}
	query.Set("complexQuery", `{"cnfUnits":[{"cnfUnit":[{"attr":"target-nf-type","value":"UDM"}]}]}`)

	if problemDetails := validateComplexQuery(query); problemDetails != nil {
		t.Fatalf("unexpected problem details: %+v", problemDetails)
	}
}

// TestNFDiscoveryProcedureRejectsRequesterNfInstanceFqdnMixedWithUnsupportedAttrInComplexQuery
// verifies end to end that NFDiscoveryProcedure rejects a complexQuery
// request combining requester-nf-instance-fqdn with an attribute
// filterByComplexQuery cannot evaluate, rather than letting the FQDN atom
// reach MongoDB's $regexMatch.
func TestNFDiscoveryProcedureRejectsRequesterNfInstanceFqdnMixedWithUnsupportedAttrInComplexQuery(t *testing.T) {
	query := url.Values{}
	query.Set("target-nf-type", nfTypeUDM)
	query.Set("requester-nf-type", nfTypeAMF)
	query.Set("complexQuery", `{"cnfUnits":[{"cnfUnit":[{"attr":"requester-nf-instance-fqdn","value":"example.com"},{"attr":"tai","value":"{}"}]}]}`)

	response, problemDetails := NFDiscoveryProcedure(query)
	if response != nil {
		t.Fatalf("expected no SearchResult when rejecting complexQuery requester-nf-instance-fqdn, got %+v", response)
	}
	if problemDetails == nil {
		t.Fatal("expected problem details rejecting requester-nf-instance-fqdn within complexQuery")
	}
}

// mockComplexQueryFqdnDiscoveryDBClient returns two profiles matching every
// Mongo-side discovery filter, including the CNF unit combining
// requester-nf-instance-fqdn with target-nf-type (matched unconditionally in
// Mongo; see fqdnAtomRequiresGoEvaluation): only requester-nf-instance-fqdn,
// applied in Go after the query by filterByComplexQuery, should tell them
// apart.
type mockComplexQueryFqdnDiscoveryDBClient struct {
	dbadapter.DBInterface
}

func (db *mockComplexQueryFqdnDiscoveryDBClient) RestfulAPIGetMany(collName string, filter bson.M) ([]map[string]interface{}, error) {
	if collName != collNfProfile {
		return nil, nil
	}
	return []map[string]interface{}{
		{
			fieldNfInstanceId: testNfInstanceUdmAllowed,
			fieldNfTypeLower:  nfTypeUDM,
			testFieldNfStatus: nfServiceStatusRegistered,
			fieldNfServices: []map[string]interface{}{{
				fieldServiceName:     testServiceNameNudmSdm,
				fieldNfServiceStatus: nfServiceStatusRegistered,
			}},
		},
		{
			fieldNfInstanceId: testNfInstanceUdmBlocked,
			fieldNfTypeLower:  nfTypeUDM,
			testFieldNfStatus: nfServiceStatusRegistered,
			fieldNfServices: []map[string]interface{}{{
				fieldServiceName:      testServiceNameNudmSdm,
				fieldNfServiceStatus:  nfServiceStatusRegistered,
				fieldAllowedNfDomains: []string{testOtherFqdn},
			}},
		},
	}, nil
}

// TestNFDiscoveryProcedureAppliesComplexQueryRequesterNfInstanceFqdnPredicate
// verifies end to end that a CNF complexQuery combining
// requester-nf-instance-fqdn with target-nf-type - matched unconditionally in
// Mongo (see fqdnAtomRequiresGoEvaluation) - is correctly narrowed by
// filterByComplexQuery afterwards, restoring the exactness the previous
// outright rejection gave up on this attribute combination.
func TestNFDiscoveryProcedureAppliesComplexQueryRequesterNfInstanceFqdnPredicate(t *testing.T) {
	originalDBClient := dbadapter.DBClient
	defer func() {
		dbadapter.DBClient = originalDBClient
	}()
	dbadapter.DBClient = &mockComplexQueryFqdnDiscoveryDBClient{}

	query := url.Values{}
	query.Set("target-nf-type", nfTypeUDM)
	query.Set("requester-nf-type", nfTypeAMF)
	query.Set("complexQuery", `{"cnfUnits":[{"cnfUnit":[{"attr":"requester-nf-instance-fqdn","value":"example.com"},{"attr":"target-nf-type","value":"AMF"}]}]}`)

	response, problemDetails := NFDiscoveryProcedure(query)
	if problemDetails != nil {
		t.Fatalf("unexpected problem details: %+v", problemDetails)
	}
	if response == nil {
		t.Fatal("expected discovery response")
	}
	if len(response.NfInstances) != 1 {
		t.Fatalf("expected 1 NF instance after complexQuery requester-nf-instance-fqdn filtering, got %d: %+v", len(response.NfInstances), response.NfInstances)
	}
	if response.NfInstances[0].NfInstanceId != testNfInstanceUdmAllowed {
		t.Fatalf("expected the unrestricted profile to be returned, got %+v", response.NfInstances[0])
	}
}

// TestMatchesComplexQueryCnfRequesterNfInstanceFqdnAllowsMissingOrMatchingPattern
// verifies that matchesComplexQuery matches a CNF unit's positive
// (non-negated) requester-nf-instance-fqdn atom against a profile whose
// service allows the FQDN (a matching allowedNfDomains pattern, or no
// allowedNfDomains at all), and rejects one whose only service restricts to a
// different pattern.
func TestMatchesComplexQueryCnfRequesterNfInstanceFqdnAllowsMissingOrMatchingPattern(t *testing.T) {
	complexQueryStruct := &models.ComplexQuery{}
	query := `{"cnfUnits":[{"cnfUnit":[{"attr":"requester-nf-instance-fqdn","value":"example.com"}]}]}`
	if err := json.Unmarshal([]byte(query), complexQueryStruct); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	allowed := models.NFProfileDiscovery{NfServices: []models.NFService{{AllowedNfDomains: []string{`^.*\.example\.com$|^example\.com$`}}}}
	if !matchesComplexQuery(allowed, complexQueryStruct) {
		t.Fatalf("expected profile with a matching allowedNfDomains pattern to match")
	}

	unrestricted := models.NFProfileDiscovery{NfServices: []models.NFService{{}}}
	if !matchesComplexQuery(unrestricted, complexQueryStruct) {
		t.Fatalf("expected profile with no allowedNfDomains to match")
	}

	blocked := models.NFProfileDiscovery{NfServices: []models.NFService{{AllowedNfDomains: []string{testOtherFqdn}}}}
	if matchesComplexQuery(blocked, complexQueryStruct) {
		t.Fatalf("expected profile whose only service restricts to a different domain not to match")
	}
}

// TestMatchesComplexQueryNegatesRequesterNfInstanceFqdn verifies that a
// negated requester-nf-instance-fqdn atom matches profiles that the
// non-negated form rejects, and vice versa.
func TestMatchesComplexQueryNegatesRequesterNfInstanceFqdn(t *testing.T) {
	complexQueryStruct := &models.ComplexQuery{}
	query := `{"cnfUnits":[{"cnfUnit":[{"attr":"requester-nf-instance-fqdn","value":"example.com","negative":true}]}]}`
	if err := json.Unmarshal([]byte(query), complexQueryStruct); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	blocked := models.NFProfileDiscovery{NfServices: []models.NFService{{AllowedNfDomains: []string{testOtherFqdn}}}}
	if !matchesComplexQuery(blocked, complexQueryStruct) {
		t.Fatalf("expected the negated atom to match a profile the positive form rejects")
	}

	allowed := models.NFProfileDiscovery{NfServices: []models.NFService{{}}}
	if matchesComplexQuery(allowed, complexQueryStruct) {
		t.Fatalf("expected the negated atom to reject a profile the positive form matches")
	}
}

// TestMatchesComplexQueryCnfCombinesFqdnWithOtherAttr verifies CNF's OR
// semantics for a unit combining requester-nf-instance-fqdn with
// target-nf-type: a profile matching either atom satisfies the unit.
func TestMatchesComplexQueryCnfCombinesFqdnWithOtherAttr(t *testing.T) {
	complexQueryStruct := &models.ComplexQuery{}
	query := `{"cnfUnits":[{"cnfUnit":[{"attr":"requester-nf-instance-fqdn","value":"example.com"},{"attr":"target-nf-type","value":"UDM"}]}]}`
	if err := json.Unmarshal([]byte(query), complexQueryStruct); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	matchesOnlyByType := models.NFProfileDiscovery{
		NfType:     models.NFTYPE_UDM,
		NfServices: []models.NFService{{AllowedNfDomains: []string{testOtherFqdn}}},
	}
	if !matchesComplexQuery(matchesOnlyByType, complexQueryStruct) {
		t.Fatalf("expected a profile matching only target-nf-type to satisfy the OR unit")
	}

	matchesNeither := models.NFProfileDiscovery{
		NfType:     models.NFTYPE_AMF,
		NfServices: []models.NFService{{AllowedNfDomains: []string{testOtherFqdn}}},
	}
	if matchesComplexQuery(matchesNeither, complexQueryStruct) {
		t.Fatalf("expected a profile matching neither atom not to satisfy the OR unit")
	}
}

// TestMatchesComplexQueryDnfCombinesFqdnWithOtherAttr verifies DNF's AND
// semantics for a unit combining requester-nf-instance-fqdn with
// target-nf-type: a profile must match both atoms to satisfy the unit.
func TestMatchesComplexQueryDnfCombinesFqdnWithOtherAttr(t *testing.T) {
	complexQueryStruct := &models.ComplexQuery{}
	query := `{"dnfUnits":[{"dnfUnit":[{"attr":"requester-nf-instance-fqdn","value":"example.com"},{"attr":"target-nf-type","value":"UDM"}]}]}`
	if err := json.Unmarshal([]byte(query), complexQueryStruct); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	matchesBoth := models.NFProfileDiscovery{NfType: models.NFTYPE_UDM, NfServices: []models.NFService{{}}}
	if !matchesComplexQuery(matchesBoth, complexQueryStruct) {
		t.Fatalf("expected a profile matching both atoms to satisfy the AND unit")
	}

	matchesOnlyType := models.NFProfileDiscovery{NfType: models.NFTYPE_UDM, NfServices: []models.NFService{{AllowedNfDomains: []string{testOtherFqdn}}}}
	if matchesComplexQuery(matchesOnlyType, complexQueryStruct) {
		t.Fatalf("expected a profile matching only one atom not to satisfy the AND unit")
	}
}

// TestMatchesComplexQueryNegatedServiceNamesMatchesMongoNinSemantics verifies
// that a negated service-names atom mirrors addServiceNamesFilter's $nin
// semantics (matchesServiceNamesNegated) rather than the boolean complement
// of matchesServiceNames: a profile with both a requested-name service and a
// different registered service matches the negated form too, since Mongo's
// $nin only requires some registered service to have a different name.
func TestMatchesComplexQueryNegatedServiceNamesMatchesMongoNinSemantics(t *testing.T) {
	complexQueryStruct := &models.ComplexQuery{}
	query := `{"cnfUnits":[{"cnfUnit":[{"attr":"service-names","value":"` + testServiceNameNudmSdm + `","negative":true},{"attr":"requester-nf-instance-fqdn","value":"example.com"}]}]}`
	if err := json.Unmarshal([]byte(query), complexQueryStruct); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	both := models.NFProfileDiscovery{NfServices: []models.NFService{
		{ServiceName: testServiceNameNudmSdm, NfServiceStatus: models.NFSERVICESTATUS_REGISTERED, AllowedNfDomains: []string{testOtherFqdn}},
		{ServiceName: "nudm-uecm", NfServiceStatus: models.NFSERVICESTATUS_REGISTERED, AllowedNfDomains: []string{testOtherFqdn}},
	}}
	if !matchesComplexQuery(both, complexQueryStruct) {
		t.Fatalf("expected a profile with both a requested-name and a different registered service to match the negated atom")
	}

	onlyRequested := models.NFProfileDiscovery{NfServices: []models.NFService{
		{ServiceName: testServiceNameNudmSdm, NfServiceStatus: models.NFSERVICESTATUS_REGISTERED, AllowedNfDomains: []string{testOtherFqdn}},
	}}
	if matchesComplexQuery(onlyRequested, complexQueryStruct) {
		t.Fatalf("expected a profile with only the requested-name service not to match the negated atom")
	}
}

// TestComplexQueryUsesOnlySupportedAttrs verifies the gate
// validateComplexQuery uses to decide whether a complexQuery containing
// requester-nf-instance-fqdn can be safely, exactly re-evaluated in Go.
func TestComplexQueryUsesOnlySupportedAttrs(t *testing.T) {
	tests := map[string]struct {
		complexQuery string
		want         bool
	}{
		"all supported": {
			`{"cnfUnits":[{"cnfUnit":[{"attr":"requester-nf-instance-fqdn","value":"example.com"},{"attr":"target-nf-type","value":"UDM"}]}]}`,
			true,
		},
		"unsupported attr": {
			`{"cnfUnits":[{"cnfUnit":[{"attr":"requester-nf-instance-fqdn","value":"example.com"},{"attr":"tai","value":"{}"}]}]}`,
			false,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			complexQueryStruct := &models.ComplexQuery{}
			if err := json.Unmarshal([]byte(tc.complexQuery), complexQueryStruct); err != nil {
				t.Fatalf("unexpected unmarshal error: %v", err)
			}
			if got := complexQueryUsesOnlySupportedAttrs(complexQueryStruct); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
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

// TestComplexQueryFilterSubprocessSkipsInvalidSnssaisValue verifies that an
// invalid snssais value is skipped, and - since it is then the only atom in
// the unit - the unit matches unconditionally rather than leaving an empty
// $and array, which MongoDB rejects.
func TestComplexQueryFilterSubprocessSkipsInvalidSnssaisValue(t *testing.T) {
	filter := complexQueryFilterSubprocess(map[string]*AtomElem{
		fieldSnssais: {value: `{"sst":1`},
	}, COMPLEX_QUERY_TYPE_DNF)

	andFilters, ok := filter[mongoOpAnd].([]bson.M)
	if !ok || len(andFilters) != 1 || len(andFilters[0]) != 0 {
		t.Fatalf("expected a single match-all alternative for an invalid snssais value, got %#v", filter[mongoOpAnd])
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
