// Copyright (c) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/omec-project/nrf/context"
	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	stats "github.com/omec-project/nrf/metrics"
	"github.com/omec-project/nrf/util"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	queryParamTargetNFType            = "target-nf-type"
	queryParamRequesterNFType         = "requester-nf-type"
	mongoOpExists                     = "$exists"
	queryParamServiceNames            = "service-names"
	mongoOpElemMatch                  = "$elemMatch"
	queryParamTargetPlmnList          = "target-plmn-list"
	queryParamTargetNfFqdn            = "target-nf-fqdn"
	queryParamNsiList                 = "nsi-list"
	queryParamSmfServingArea          = "smf-serving-area"
	errUnmarshalTaiByteArray          = "marshal/unmarshal error in taiByteArray:"
	queryParamAmfRegionID             = "amf-region-id"
	queryParamAmfSetID                = "amf-set-id"
	errUnmarshalGuamiByteArray        = "marshal/unmarshal error in guamiByteArray:"
	fieldUdmInfoSupiRanges            = "udminfo.supiranges"
	fieldUdmInfoGpsiRanges            = "udminfo.gpsiranges"
	fieldUdmExtGrpIDRanges            = "udminfo.externalgroupidentifiersranges"
	fieldUdrInfoSupiRanges            = "udrinfo.supiranges"
	fieldUdrInfoGpsiRanges            = "udrinfo.gpsiranges"
	fieldUdrExtGroupIDRanges          = "udrinfo.externalgroupidentifiersranges"
	fieldPcfInfoSupiRanges            = "pcfinfo.supiranges"
	queryParamUeIpv4Address           = "ue-ipv4-address"
	queryParamIpDomain                = "ip-domain"
	queryParamUeIpv6Prefix            = "ue-ipv6-prefix"
	queryParamPgwInd                  = "pgw-ind"
	queryParamExternalGroupIdentity   = "external-group-identity"
	queryParamDataSet                 = "data-set"
	queryParamRoutingIndicator        = "routing-indicator"
	queryParamGroupIDList             = "group-id-list"
	queryParamDnaiList                = "dnai-list"
	queryParamUpfIwkEpsInd            = "upf-iwk-eps-ind"
	queryParamChfSupportedPlmn        = "chf-supported-plmn"
	fieldChfInfoPlmnRangeList         = "chfinfo.plmnrangelist"
	queryParamPreferredLocality       = "preferred-locality"
	queryParamAccessType              = "access-type"
	queryParamSupportedFeatures       = "supported-features"
	queryParamRequesterNfInstanceFqdn = "requester-nfinstance-fqdn"
	queryParamTargetNfInstanceID      = "target-nf-instanceid"
	queryParamDnn                     = "dnn"

	mongoOpOr  = "$or"
	mongoOpAnd = "$and"
	mongoOpIn  = "$in"
	mongoOpLte = "$lte"
	mongoOpGte = "$gte"
	mongoOpNot = "$not"
	mongoOpNe  = "$ne"

	nfTypeAMF     = "AMF"
	nfTypeSMF     = "SMF"
	nfTypeUPF     = "UPF"
	nfTypePCF     = "PCF"
	nfTypeBSF     = "BSF"
	nfTypeCHF     = "CHF"
	nfTypeAUSF    = "AUSF"
	nfTypeUDM     = "UDM"
	nfTypeUDR     = "UDR"
	nfTypeUnknown = "UNKNOWN_NF"

	nfServiceStatusRegistered = "REGISTERED"

	fieldBsfInfo          = "bsfinfo"
	fieldAmfInfo          = "amfinfo"
	fieldChfInfo          = "chfinfo"
	fieldAusfInfo         = "ausfinfo"
	fieldDnnUpfInfoList   = "dnnupfinfolist"
	fieldAllowedNfDomains = "allowednfdomains"

	collNfProfile     = "NfProfile"
	collSubscriptions = "Subscriptions"
	collUriList       = "urilist"

	fieldNfType          = "nfType"
	fieldNfTypeLower     = "nftype"
	fieldNfInstanceId    = "nfinstanceid"
	fieldSubscriptionId  = "subscriptionId"
	fieldNfServices      = "nfservices"
	fieldServiceName     = "servicename"
	fieldNfServiceStatus = "nfservicestatus"
	fieldPlmnList        = "plmnlist"
	fieldFqdn            = "fqdn"
	fieldStart           = "start"
	fieldEnd             = "end"
	fieldUpfInfo         = "upfinfo"
	fieldSmfInfo         = "smfinfo"
	fieldSupiRanges      = "supiranges"
	fieldUdmInfo         = "udminfo"
	fieldUdrInfo         = "udrinfo"
	fieldGpsiRangeList   = "gpsirangelist"
	fieldGroupId         = "groupid"
	fieldSnssais         = "snssais"
	fieldExpireAt        = "expireAt"
)

// rawExpireAtToTime converts an expireAt value from a raw MongoDB document to
// time.Time. The value may be bson.DateTime (returned by the driver when
// decoding into an `any`) or time.Time (if the field was set directly in an
// in-memory map before being persisted). Returns false when the conversion is
// not possible.
func rawExpireAtToTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case bson.DateTime:
		return t.Time(), true
	case time.Time:
		return t, true
	}
	return time.Time{}, false
}

func cloneDiscoveryQueryParameters(queryParameters url.Values) url.Values {
	cloned := make(url.Values, len(queryParameters))
	for key, values := range queryParameters {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func hasExplodedDiscoveryQueryParam(queryParameters url.Values, prefix string) bool {
	for key := range queryParameters {
		if strings.HasPrefix(key, prefix+"[") {
			return true
		}
	}
	return false
}

func normalizeDiscoveryQueryParameters(queryParameters url.Values) url.Values {
	normalized := cloneDiscoveryQueryParameters(queryParameters)

	if normalized.Get(queryParamTargetPlmnList) == "" && hasExplodedDiscoveryQueryParam(normalized, queryParamTargetPlmnList) {
		if value, ok := marshalExplodedPlmnIDList(normalized, queryParamTargetPlmnList); ok {
			normalized.Set(queryParamTargetPlmnList, value)
		}
	}
	if normalized.Get(fieldSnssais) == "" && hasExplodedDiscoveryQueryParam(normalized, fieldSnssais) {
		if value, ok := marshalExplodedSnssaiList(normalized, fieldSnssais); ok {
			normalized.Set(fieldSnssais, value)
		}
	}
	if normalized.Get("tai") == "" && hasExplodedDiscoveryQueryParam(normalized, "tai") {
		if value, ok := marshalExplodedTai(normalized, "tai"); ok {
			normalized.Set("tai", value)
		}
	}
	if normalized.Get("guami") == "" && hasExplodedDiscoveryQueryParam(normalized, "guami") {
		if value, ok := marshalExplodedGuami(normalized, "guami"); ok {
			normalized.Set("guami", value)
		}
	}

	return normalized
}

func marshalExplodedPlmnIDList(queryParameters url.Values, prefix string) (string, bool) {
	mccValues := queryParameters[prefix+"[mcc]"]
	mncValues := queryParameters[prefix+"[mnc]"]
	count := max(len(mccValues), len(mncValues))
	if count == 0 {
		return "", false
	}

	encoded := make([]string, 0, count)
	for index := range count {
		plmnID := models.NewPlmnIdWithDefaults()
		if index < len(mccValues) {
			plmnID.SetMcc(mccValues[index])
		}
		if index < len(mncValues) {
			plmnID.SetMnc(mncValues[index])
		}
		marshaled, err := json.Marshal(plmnID)
		if err != nil {
			logger.DiscoveryLog.Warnln("marshal error in exploded plmnID:", err)
			return "", false
		}
		encoded = append(encoded, string(marshaled))
	}

	return strings.Join(encoded, ","), true
}

func marshalExplodedSnssaiList(queryParameters url.Values, prefix string) (string, bool) {
	sstValues := queryParameters[prefix+"[sst]"]
	sdValues := queryParameters[prefix+"[sd]"]
	count := max(len(sstValues), len(sdValues))
	if count == 0 {
		return "", false
	}

	encoded := make([]string, 0, count)
	for index := range count {
		snssai := models.NewSnssaiWithDefaults()
		if index < len(sstValues) && sstValues[index] != "" {
			sstValue, err := strconv.ParseInt(sstValues[index], 10, 32)
			if err != nil {
				logger.DiscoveryLog.Warnln("parse error in exploded snssai sst:", err)
				return "", false
			}
			snssai.SetSst(int32(sstValue))
		}
		if index < len(sdValues) && sdValues[index] != "" {
			snssai.SetSd(sdValues[index])
		}
		marshaled, err := json.Marshal(snssai)
		if err != nil {
			logger.DiscoveryLog.Warnln("marshal error in exploded snssai:", err)
			return "", false
		}
		encoded = append(encoded, string(marshaled))
	}

	return strings.Join(encoded, ","), true
}

func marshalExplodedTai(queryParameters url.Values, prefix string) (string, bool) {
	plmnID := models.NewPlmnId(queryParameters.Get(prefix+"[plmnId][mcc]"), queryParameters.Get(prefix+"[plmnId][mnc]"))
	tac := queryParameters.Get(prefix + "[tac]")
	nid := queryParameters.Get(prefix + "[nid]")
	if plmnID.GetMcc() == "" && plmnID.GetMnc() == "" && tac == "" && nid == "" {
		return "", false
	}

	tai := models.NewTai(*plmnID, tac)
	if nid != "" {
		tai.SetNid(nid)
	}

	marshaled, err := json.Marshal(tai)
	if err != nil {
		logger.DiscoveryLog.Warnln("marshal error in exploded tai:", err)
		return "", false
	}
	return string(marshaled), true
}

func marshalExplodedGuami(queryParameters url.Values, prefix string) (string, bool) {
	plmnID := models.NewPlmnIdNid(queryParameters.Get(prefix+"[plmnId][mcc]"), queryParameters.Get(prefix+"[plmnId][mnc]"))
	if nid := queryParameters.Get(prefix + "[plmnId][nid]"); nid != "" {
		plmnID.SetNid(nid)
	}
	amfID := queryParameters.Get(prefix + "[amfId]")
	if plmnID.GetMcc() == "" && plmnID.GetMnc() == "" && plmnID.GetNid() == "" && amfID == "" {
		return "", false
	}

	guami := models.NewGuami(*plmnID, amfID)
	marshaled, err := json.Marshal(guami)
	if err != nil {
		logger.DiscoveryLog.Warnln("marshal error in exploded guami:", err)
		return "", false
	}
	return string(marshaled), true
}

func splitTopLevelCommaSeparatedJSONValues(raw string) []string {
	values := make([]string, 0, 1)
	start := 0
	depth := 0
	inString := false
	escaped := false

	for index, char := range raw {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && inString {
			escaped = true
			continue
		}
		if char == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}

		switch char {
		case '{', '[':
			depth++
		case '}', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				value := strings.TrimSpace(raw[start:index])
				if value != "" {
					values = append(values, value)
				}
				start = index + 1
			}
		}
	}

	if tail := strings.TrimSpace(raw[start:]); tail != "" {
		values = append(values, tail)
	}

	return values
}

func buildSnssaisElemMatchFilters(raw string) []bson.M {
	encodedValues := splitTopLevelCommaSeparatedJSONValues(raw)
	filters := make([]bson.M, 0, len(encodedValues))

	for _, encodedValue := range encodedValues {
		snssaiStruct := models.NewSnssaiWithDefaults()
		err := json.Unmarshal([]byte(encodedValue), snssaiStruct)
		if err != nil {
			logger.DiscoveryLog.Warnln("unmarshal error in snssaiStruct:", err)
			continue
		}

		snssaiByteArray, err := bson.Marshal(snssaiStruct)
		if err != nil {
			logger.DiscoveryLog.Warnln("marshal error in snssaiStruct:", err)
			continue
		}

		snssaiBsonM := bson.M{}
		err = bson.Unmarshal(snssaiByteArray, &snssaiBsonM)
		if err != nil {
			logger.DiscoveryLog.Warnln("unmarshal error in snssaiBsonM:", err)
			continue
		}
		for key, value := range snssaiBsonM {
			if value == nil {
				delete(snssaiBsonM, key)
			}
		}

		filters = append(filters, bson.M{
			fieldSnssais: bson.M{
				mongoOpElemMatch: snssaiBsonM,
			},
		})
	}

	return filters
}

func HandleNFDiscoveryRequest(request *httpwrapper.Request) *httpwrapper.Response {
	// Get all query parameters
	logger.DiscoveryLog.Infoln("Handle NFDiscoveryRequest")

	response, problemDetails := NFDiscoveryProcedure(request.Query)
	requesterNfType, targetNfType := GetRequesterAndTargetNfTypeGivenQueryParameters(request.Query)
	// Send Response
	// step 4: process the return value from step 3
	if response != nil {
		// status code is based on SPEC, and option headers
		stats.IncrementNrfNfInstancesStats(requesterNfType, targetNfType, "SUCCESS")
		return httpwrapper.NewResponse(http.StatusOK, nil, response)
	} else if problemDetails != nil {
		stats.IncrementNrfNfInstancesStats(requesterNfType, targetNfType, "FAILURE")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	problemDetails = utils.ProblemDetailsUnspecified()
	stats.IncrementNrfNfInstancesStats(requesterNfType, targetNfType, "FAILURE")
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

func NFDiscoveryProcedure(queryParameters url.Values) (response *models.SearchResult,
	problemDetails *models.ProblemDetails,
) {
	queryParameters = normalizeDiscoveryQueryParameters(queryParameters)

	if queryParameters[queryParamTargetNFType] == nil || queryParameters[queryParamRequesterNFType] == nil {
		problemDetails = utils.ProblemDetailsWithCause("Invalid Parameter", http.StatusBadRequest, "Missing mandatory parameter", utils.CauseMandatoryIeMissing)
		return nil, problemDetails
	}

	if problem := validateComplexQuery(queryParameters); problem != nil {
		return nil, problem
	}

	// Check ComplexQuery (FOR REPORT PROBLEM!)

	// Build Query Filter
	filter := buildFilter(queryParameters)
	logger.DiscoveryLog.Debugln("query filter:", filter)

	// Use the filter to find documents
	nfProfilesRaw, err := dbadapter.DBClient.RestfulAPIGetMany(collNfProfile, filter)
	if err != nil {
		logger.DiscoveryLog.Warnln("NF profile query error:", err)
	}
	logger.DiscoveryLog.Debugf("primary discovery raw count: %d", len(nfProfilesRaw))

	// sort nfprofiles based on expiry timestamp before decoding so that the
	// ordering is reflected in the returned SearchResult.
	// Sort profiles
	nfProfilesStruct := sortNFProfiles(nfProfilesRaw, queryParameters)

	// Handle IPv4 & IPv6 conversion for BSF profiles
	handleBSFIpConversion(queryParameters, nfProfilesStruct)

	// Build SearchResult model
	searchResult := models.NewSearchResult(100, nfProfilesStruct)

	return searchResult, nil
}

func validateComplexQuery(queryParameters url.Values) *models.ProblemDetails {
	if values := queryParameters["complexQuery"]; len(values) > 0 {
		// IF SUPPORT COMPLEX QUERY
		// translate raw data to complexQuery structure
		complexQuery := values[0]
		complexQueryStruct := &models.ComplexQuery{}
		err := json.Unmarshal([]byte(complexQuery), complexQueryStruct)
		if err != nil {
			logger.DiscoveryLog.Warnln("unmarshal complexQuery Error:", err)
		}
		// Check either CNF or DNF
		if complexQueryStruct.Cnf != nil && complexQueryStruct.Dnf != nil {
			problemDetails := utils.ProblemDetailsWithCause("Invalid Parameter", http.StatusBadRequest, "CNF and DNF are mutually exclusive", utils.CauseInvalidRequest)
			problemDetails.SetInvalidParams([]models.InvalidParam{
				{Param: "complexQuery"},
			})
			return problemDetails
		}
	}
	return nil
}

func sortNFProfiles(
	nfProfilesRaw []map[string]interface{},
	queryParameters url.Values,
) []models.NFProfileDiscovery {
	sort.Slice(nfProfilesRaw, func(i, j int) bool {
		timeI, okI := rawExpireAtToTime(nfProfilesRaw[i][fieldExpireAt])
		timeJ, okJ := rawExpireAtToTime(nfProfilesRaw[j][fieldExpireAt])
		if okI != okJ {
			return okI // profiles with an expireAt sort before those without
		}
		if !okI {
			return false // both missing expireAt: treat as equal
		}
		return timeI.Before(timeJ)
	})

	// nfProfile data for response
	var nfProfilesStruct []models.NFProfileDiscovery

	nfProfilesStruct, err := util.Decode(nfProfilesRaw, time.RFC3339)
	if err != nil {
		logger.DiscoveryLog.Warnln("NF Profile Raw decode error:", err)
	}
	logger.DiscoveryLog.Debugf("primary discovery decoded count: %d", len(nfProfilesStruct))

	// Populate cache so subsequent fallback calls can skip MongoDB+decode.
	for i, p := range nfProfilesStruct {
		var rawDoc map[string]any
		if i < len(nfProfilesRaw) {
			rawDoc = nfProfilesRaw[i]
		}
		cacheProfileWithExpiry(p, rawDoc)
	}

	if len(nfProfilesStruct) == 0 {
		allProfiles, fallbackErr := loadDiscoveryProfilesFromURIList(queryParameters)
		if fallbackErr != nil {
			logger.DiscoveryLog.Warnln("fallback discovery load error:", fallbackErr)
		} else {
			logger.DiscoveryLog.Debugf("fallback discovery decoded count: %d", len(allProfiles))
			nfProfilesStruct = filterDiscoveryResults(allProfiles, queryParameters)
			logger.DiscoveryLog.Debugf("fallback filtered count: %d", len(nfProfilesStruct))
		}
	}
	return nfProfilesStruct
}

func handleBSFIpConversion(
	queryParameters url.Values,
	nfProfilesStruct []models.NFProfileDiscovery,
) {
	if queryParameters[queryParamTargetNFType][0] == nfTypeBSF {
		for i, nfProfile := range nfProfilesStruct {
			if nfProfile.BsfInfo == nil {
				continue
			}
			ipv4AddressRanges, ok := nfProfile.BsfInfo.GetIpv4AddressRangesOk()
			if ok {
				for j, ipv4AddressRange := range ipv4AddressRanges {
					ipv4IntStart, err := strconv.Atoi(ipv4AddressRange.GetStart())
					if err != nil {
						logger.DiscoveryLog.Warnln("ipv4IntStart Atoi Error:", err)
					}
					(((*nfProfilesStruct[i].BsfInfo).Ipv4AddressRanges)[j]).Start = context.Ipv4IntToIpv4String(int64(ipv4IntStart))
					ipv4IntEnd, err := strconv.Atoi(ipv4AddressRange.GetEnd())
					if err != nil {
						logger.DiscoveryLog.Warnln("ipv4IntEnd Atoi Error:", err)
					}
					(((*nfProfilesStruct[i].BsfInfo).Ipv4AddressRanges)[j]).End = context.Ipv4IntToIpv4String(int64(ipv4IntEnd))
				}
			}
			ipv6PrefixRanges, ok := nfProfile.BsfInfo.GetIpv6PrefixRangesOk()
			if ok {
				for j, ipv6PrefixRange := range ipv6PrefixRanges {
					ipv6IntStart := new(big.Int)
					ipv6IntStart.SetString(ipv6PrefixRange.GetStart(), 10)
					(((*nfProfilesStruct[i].BsfInfo).Ipv6PrefixRanges)[j]).Start = context.Ipv6IntToIpv6String(ipv6IntStart)

					ipv6IntEnd := new(big.Int)
					ipv6IntEnd.SetString(ipv6PrefixRange.GetEnd(), 10)
					(((*nfProfilesStruct[i].BsfInfo).Ipv6PrefixRanges)[j]).End = context.Ipv6IntToIpv6String(ipv6IntEnd)
				}
			}
		}
	}
}

func loadDiscoveryProfilesFromURIList(queryParameters url.Values) ([]models.NFProfileDiscovery, error) {
	targetNfType := queryParameters[queryParamTargetNFType][0]
	uriListRaw, err := dbadapter.DBClient.RestfulAPIGetOne(collUriList, bson.M{fieldNfType: targetNfType})
	if err != nil {
		return nil, err
	}
	if uriListRaw == nil {
		return nil, nil
	}

	uriList := &context.UriList{}
	err = mapstructure.Decode(uriListRaw, uriList)
	if err != nil {
		return nil, err
	}

	logger.DiscoveryLog.Debugf("fallback urilist count: %d", len(uriList.Link.Item))

	orderedInstanceIDs := make([]string, 0, len(uriList.Link.Item))
	uniqueInstanceIDs := make([]string, 0, len(uriList.Link.Item))
	seenInstanceIDs := make(map[string]struct{}, len(uriList.Link.Item))
	for _, item := range uriList.Link.Item {
		nfInstanceID := getNFInstanceIDFromURI(item.GetHref())
		if nfInstanceID == "" {
			continue
		}
		orderedInstanceIDs = append(orderedInstanceIDs, nfInstanceID)
		if _, seen := seenInstanceIDs[nfInstanceID]; seen {
			continue
		}
		seenInstanceIDs[nfInstanceID] = struct{}{}
		uniqueInstanceIDs = append(uniqueInstanceIDs, nfInstanceID)
	}

	if len(uniqueInstanceIDs) == 0 {
		return nil, nil
	}

	// Serve cached profiles; only fetch the remainder from MongoDB.
	decodedByID := make(map[string]models.NFProfileDiscovery, len(uniqueInstanceIDs))
	uncachedIDs := make([]string, 0, len(uniqueInstanceIDs))
	for _, id := range uniqueInstanceIDs {
		if p, ok := profileCache.get(id); ok {
			decodedByID[id] = p
		} else {
			uncachedIDs = append(uncachedIDs, id)
		}
	}

	if len(uncachedIDs) > 0 {
		profileListRaw, dbErr := dbadapter.DBClient.RestfulAPIGetMany(collNfProfile, bson.M{
			fieldNfInstanceId: bson.M{mongoOpIn: uncachedIDs},
		})
		if dbErr != nil {
			return nil, dbErr
		}

		profilesByInstanceID := make(map[string]map[string]interface{}, len(profileListRaw))
		for _, profileRaw := range profileListRaw {
			if profileRaw == nil {
				continue
			}
			if nfInstanceID, ok := profileRaw[fieldNfInstanceId].(string); ok && nfInstanceID != "" {
				profilesByInstanceID[nfInstanceID] = profileRaw
			}
		}

		// Batch-decode all uncached profiles in a single util.Decode call.
		rawBatch := make([]any, 0, len(uncachedIDs))
		for _, nfInstanceID := range uncachedIDs {
			if profileRaw := profilesByInstanceID[nfInstanceID]; profileRaw != nil {
				rawBatch = append(rawBatch, profileRaw)
			}
		}

		if len(rawBatch) > 0 {
			decoded, decodeErr := util.Decode(rawBatch, time.RFC3339)
			if decodeErr != nil {
				// Fall back to per-document decode so one malformed entry doesn't
				// discard the entire batch (mirrors the original per-profile loop).
				logger.DiscoveryLog.Warnf("fallback profile batch decode error, retrying per-profile: %v", decodeErr)
				for _, raw := range rawBatch {
					rawDoc, _ := raw.(map[string]any)
					single, sErr := util.Decode([]any{raw}, time.RFC3339)
					if sErr != nil {
						logger.DiscoveryLog.Warnf("fallback profile decode error: %v", sErr)
						continue
					}
					if len(single) == 0 || single[0].GetNfInstanceId() == "" {
						continue
					}
					p := single[0]
					decodedByID[p.GetNfInstanceId()] = p
					cacheProfileWithExpiry(p, rawDoc)
				}
			} else {
				for i, p := range decoded {
					if p.GetNfInstanceId() == "" {
						continue
					}
					decodedByID[p.GetNfInstanceId()] = p
					rawDoc, _ := rawBatch[i].(map[string]any)
					cacheProfileWithExpiry(p, rawDoc)
				}
			}
		}
	}

	profiles := make([]models.NFProfileDiscovery, 0, len(orderedInstanceIDs))
	for _, nfInstanceID := range orderedInstanceIDs {
		if p, ok := decodedByID[nfInstanceID]; ok {
			profiles = append(profiles, p)
		}
	}

	return profiles, nil
}

// cacheProfileWithExpiry stores p in the profile cache, deriving TTL from the
// raw document's expireAt field or the NRF keep-alive configuration.
func cacheProfileWithExpiry(p models.NFProfileDiscovery, rawDoc map[string]any) {
	var expiresAt time.Time
	if rawDoc != nil {
		if t, ok := rawExpireAtToTime(rawDoc[fieldExpireAt]); ok && t.After(time.Now()) {
			expiresAt = t
		}
	}
	if expiresAt.IsZero() {
		ttl := time.Duration(factory.NrfConfig.Configuration.NfKeepAliveTime) * time.Second
		if ttl <= 0 {
			ttl = 60 * time.Second
		}
		expiresAt = time.Now().Add(ttl)
	}
	profileCache.set(p, expiresAt)
}

func getNFInstanceIDFromURI(uri string) string {
	idx := strings.LastIndex(uri, "/")
	if idx == -1 || idx == len(uri)-1 {
		return ""
	}
	return uri[idx+1:]
}

func filterDiscoveryResults(nfProfiles []models.NFProfileDiscovery, queryParameters url.Values) []models.NFProfileDiscovery {
	filtered := make([]models.NFProfileDiscovery, 0, len(nfProfiles))
	for _, profile := range nfProfiles {
		if matchesDiscoveryQuery(profile, queryParameters) {
			filtered = append(filtered, profile)
		}
	}
	return filtered
}

func matchesDiscoveryQuery(profile models.NFProfileDiscovery, queryParameters url.Values) bool {
	if values := queryParameters[queryParamTargetNFType]; len(values) > 0 && values[0] != "" {
		if string(profile.GetNfType()) != values[0] {
			return false
		}
	}

	if values := queryParameters["target-nf-instance-id"]; len(values) > 0 && values[0] != "" {
		if profile.GetNfInstanceId() != values[0] {
			return false
		}
	}

	if values := queryParameters[queryParamRequesterNFType]; len(values) > 0 && values[0] != "" {
		allowedTypes, ok := profile.GetAllowedNfTypesOk()
		if ok && len(allowedTypes) > 0 {
			matched := false
			for _, allowedType := range allowedTypes {
				if string(allowedType) == values[0] {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}

	if values := queryParameters[queryParamServiceNames]; len(values) > 0 && values[0] != "" {
		requestedServices := strings.Split(values[0], ",")
		matched := false
		for _, service := range profile.NfServices {
			if service.NfServiceStatus != models.NFSERVICESTATUS_REGISTERED {
				continue
			}
			for _, requestedService := range requestedServices {
				if string(service.ServiceName) == requestedService {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

func buildFilter(queryParameters url.Values) bson.M {
	// build the filter
	filter := bson.M{
		mongoOpAnd: []bson.M{},
	}

	targetNfType := ""
	if values := queryParameters[queryParamTargetNFType]; len(values) > 0 {
		targetNfType = values[0]
	}

	handleTargetNfType(queryParameters, filter)
	handleRequesterNfType(queryParameters, filter)
	handleServiceNames(queryParameters, filter)
	handleRequesterNfInstanceFqdn(queryParameters, filter)
	handleTargetPlmnList(queryParameters, filter)
	handleTargetNfInstanceID(queryParameters, filter)
	handleTargetNfFqdn(queryParameters, filter)
	handleSnssais(queryParameters, filter)
	handleNsiList(queryParameters, filter)
	handleDnn(queryParameters, filter, targetNfType)
	handleSmfServingArea(queryParameters, filter, targetNfType)
	handleTai(queryParameters, filter, targetNfType)
	handleAmfRegionID(queryParameters, filter, targetNfType)
	handleAmfSetID(queryParameters, filter, targetNfType)
	handleGuami(queryParameters, filter, targetNfType)
	handleSupi(queryParameters, filter, targetNfType)
	handleUeIpv4(queryParameters, filter, targetNfType)
	handleIpDomain(queryParameters, filter, targetNfType)
	handleUeIpv6Prefix(queryParameters, filter, targetNfType)
	handlePgwInd(queryParameters, filter)
	handlePgw(queryParameters, filter)
	handleGpsi(queryParameters, filter, targetNfType)
	handleExternalGroupIdentity(queryParameters, filter, targetNfType)
	handleDataSet(queryParameters, filter, targetNfType)
	handleRoutingIndicator(queryParameters, filter, targetNfType)
	handleGroupIDList(queryParameters, filter, targetNfType)
	handleDnaiList(queryParameters, filter, targetNfType)
	handleUpfIwkEpsInd(queryParameters, filter, targetNfType)
	handleChfSupportedPlmn(queryParameters, filter, targetNfType)
	handlePreferredLocality(queryParameters, filter)
	handleAccessType(queryParameters, filter)
	handleSupportedFeatures(queryParameters, filter)
	handleComplexQuery(queryParameters, filter)

	return filter
}

func handleTargetNfType(queryParameters url.Values, filter bson.M) {
	// [Query-1] target-nf-type
	if len(queryParameters[queryParamTargetNFType]) > 0 {
		targetNfType := queryParameters[queryParamTargetNFType][0]
		if targetNfType != "" {
			targetNfTypeFilter := bson.M{
				fieldNfTypeLower: targetNfType,
			}
			filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), targetNfTypeFilter)
		}
	}
}

func handleRequesterNfType(queryParameters url.Values, filter bson.M) {
	// [Query-2] request-nf-type
	requesterNfType := queryParameters[queryParamRequesterNFType][0]
	if requesterNfType != "" {
		requesterNfTypeFilter := bson.M{
			mongoOpOr: []bson.M{
				{"allowednftypes": requesterNfType},
				{"allowednftypes": nil},
			},
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), requesterNfTypeFilter)
	}
}

func handleServiceNames(queryParameters url.Values, filter bson.M) {
	// [Query-3] service-names
	// TODO: return exist service name
	if queryParameters[queryParamServiceNames] != nil {
		serviceNames := queryParameters[queryParamServiceNames][0]
		serviceNamesSplit := strings.Split(serviceNames, ",")
		var serviceNamesBsonArray bson.A

		for _, v := range serviceNamesSplit {
			serviceNamesBsonArray = append(serviceNamesBsonArray, v)
		}
		serviceNamesFilter := bson.M{
			fieldNfServices: bson.M{
				mongoOpElemMatch: bson.M{
					fieldServiceName: bson.M{
						// get all service in array
						mongoOpIn: serviceNamesBsonArray,
					},
					// the service need to be registered
					fieldNfServiceStatus: nfServiceStatusRegistered,
				},
			},
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), serviceNamesFilter)
	}
}

func handleRequesterNfInstanceFqdn(queryParameters url.Values, filter bson.M) {
	// [Query-4] requester-nfinstance-fqdn
	if queryParameters["requester-nf-instance-fqdn"] != nil {
		requesterNfinstanceFqdn := queryParameters["requester-nf-instance-fqdn"][0]

		requesterNfinstanceFqdnFilter := bson.M{
			mongoOpOr: []bson.M{
				{
					fieldNfServices: bson.M{
						mongoOpElemMatch: bson.M{
							fieldAllowedNfDomains: requesterNfinstanceFqdn,
						},
					},
				},
				{ // if not provided, allow any.
					fieldNfServices: bson.M{
						mongoOpElemMatch: bson.M{
							fieldAllowedNfDomains: bson.M{
								mongoOpExists: false,
							},
						},
					},
				},
			},
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), requesterNfinstanceFqdnFilter)
	}
}

func handleTargetPlmnList(queryParameters url.Values, filter bson.M) {
	// [Query-5] target-plmn-list [C] = Mcc + Mnc
	// Mcc: Pattern: '^[0-9]{3}$'
	// Mnc: Pattern: '^[0-9]{2,3}$'
	if queryParameters[queryParamTargetPlmnList] != nil {
		targetPlmnList := queryParameters[queryParamTargetPlmnList][0]
		targetPlmnListSplit := strings.Split(targetPlmnList, ",")
		var targetPlmnListBsonArray bson.A

		var temptargetPlmn string
		for i, v := range targetPlmnListSplit {
			if i%2 == 0 {
				temptargetPlmn = v
			} else {
				temptargetPlmn += ","
				temptargetPlmn += v

				targetPlmnListtruct := models.NewPlmnIdWithDefaults()
				err := json.Unmarshal([]byte(temptargetPlmn), targetPlmnListtruct)
				if err != nil {
					logger.DiscoveryLog.Warnln("unmarshal error in targetPlmnListtruct:", err)
				}

				targetPlmnByteArray, err := bson.Marshal(targetPlmnListtruct)
				if err != nil {
					logger.DiscoveryLog.Warnln("marshal error in targetPlmnListtruct:", err)
				}

				targetPlmnBsonM := bson.M{}
				err = bson.Unmarshal(targetPlmnByteArray, &targetPlmnBsonM)
				if err != nil {
					logger.DiscoveryLog.Errorln("unmarshal error in targetPlmnBsonM:", err)
				}
				logger.DiscoveryLog.Debugln("temp target Plmn:", temptargetPlmn)

				targetPlmnListBsonArray = append(targetPlmnListBsonArray, bson.M{fieldPlmnList: bson.M{mongoOpElemMatch: targetPlmnBsonM}})
			}
		}

		targetPlmnListFilter := bson.M{
			mongoOpOr: targetPlmnListBsonArray,
		}

		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), targetPlmnListFilter)
	}
	// [Query-6] requester-plmn-list
	// if queryParameters["requester-plmn-list"] != nil {
	// requesterPlmnPist := queryParameters["requester-plmn-list"][0]
	// TODO
	// }
}

func handleTargetNfInstanceID(queryParameters url.Values, filter bson.M) {
	// [Query-7] target-nf-instance-id
	if queryParameters["target-nf-instance-id"] != nil {
		targetNfInstanceid := queryParameters["target-nf-instance-id"][0]
		nfInstanceIdFilter := bson.M{
			fieldNfInstanceId: targetNfInstanceid,
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), nfInstanceIdFilter)
	}
}

func handleTargetNfFqdn(queryParameters url.Values, filter bson.M) {
	// [Query-8] target-nf-fqdn
	if queryParameters[queryParamTargetNfFqdn] != nil {
		targetNfFqdn := queryParameters[queryParamTargetNfFqdn][0]
		fqdnFilter := bson.M{
			fieldFqdn: targetNfFqdn,
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), fqdnFilter)
	}
}

// [Query-9] hnrf-uri
// for Roaming
func handleSnssais(queryParameters url.Values, filter bson.M) {
	// [Query-10] snssais
	// Pattern: '^[A-Fa-f0-9]{6}$'
	if queryParameters[fieldSnssais] != nil {
		snssais := queryParameters[fieldSnssais][0]
		snssaisFilters := buildSnssaisElemMatchFilters(snssais)
		if len(snssaisFilters) > 0 {
			var snssaisBsonArray bson.A
			for _, snssaisFilter := range snssaisFilters {
				snssaisBsonArray = append(snssaisBsonArray, snssaisFilter)
			}

			// if not assign, serve all NF
			snssaisBsonArray = append(snssaisBsonArray, bson.M{fieldSnssais: bson.M{mongoOpExists: false}})

			snssaisFilter := bson.M{
				mongoOpOr: snssaisBsonArray,
			}

			filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), snssaisFilter)
		}
	}
}

func handleNsiList(queryParameters url.Values, filter bson.M) {
	// [Query-11] nsi-list
	if queryParameters[queryParamNsiList] != nil {
		nsiList := queryParameters[queryParamNsiList][0]
		nsiListSplit := strings.Split(nsiList, ",")
		var nsiListBsonArray bson.A
		for _, v := range nsiListSplit {
			nsiListBsonArray = append(nsiListBsonArray, v)
		}
		nsiListFilter := bson.M{
			"nsilist": bson.M{
				"$all": nsiListBsonArray,
			},
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), nsiListFilter)
	}
}

func handleDnn(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-12] dnn
	if queryParameters[queryParamDnn] != nil {
		dnn := queryParameters[queryParamDnn][0]
		var dnnFilter bson.M
		switch targetNfType {
		case nfTypeSMF:
			dnnFilter = bson.M{
				"smfinfo.snssaismfinfolist": bson.M{
					mongoOpElemMatch: bson.M{
						"dnnsmfinfolist": bson.M{
							mongoOpElemMatch: bson.M{
								mongoOpOr: []bson.M{
									{queryParamDnn: dnn},
									{"dnn.string": dnn},
								},
							},
						},
					},
				},
			}
		case nfTypeUPF:
			dnnFilter = bson.M{
				"upfinfo.snssaiupfinfolist": bson.M{
					mongoOpElemMatch: bson.M{
						fieldDnnUpfInfoList: bson.M{
							mongoOpElemMatch: bson.M{
								queryParamDnn: dnn,
							},
						},
					},
				},
			}
		case nfTypeBSF:
			dnnFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						"bsfinfo.dnnlist": dnn,
					},
					{
						"bsfinfo.dnnlist": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		case nfTypePCF:
			dnnFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						"pcfinfo.dnnlist": dnn,
					},
					{
						"pcfinfo.dnnlist": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), dnnFilter)
	}
}

func handleSmfServingArea(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-13] smf-serving-area
	if queryParameters[queryParamSmfServingArea] != nil {
		var smfServingAreaFilter bson.M
		smfServingArea := queryParameters[queryParamSmfServingArea][0]
		if targetNfType == nfTypeUPF {
			smfServingAreaFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						"upfinfo.smfservingarea": smfServingArea,
					},
					{
						"upfinfo.smfservingarea": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), smfServingAreaFilter)
	}
}

func handleTai(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-14] tai
	if queryParameters["tai"] != nil {
		var taiFilter bson.M
		tai := queryParameters["tai"][0]

		taiStruct := models.NewTaiWithDefaults()
		err := json.Unmarshal([]byte(tai), taiStruct)
		if err != nil {
			logger.DiscoveryLog.Warnln("unmarshal error in taiStruct:", err)
		}

		taiByteArray, err := bson.Marshal(taiStruct)
		if err != nil {
			logger.DiscoveryLog.Warnln(errUnmarshalTaiByteArray, err)
		}

		taiBsonM := bson.M{}
		err = bson.Unmarshal(taiByteArray, &taiBsonM)
		if err != nil {
			logger.DiscoveryLog.Warnln(errUnmarshalTaiByteArray, err)
		}
		switch targetNfType {
		case nfTypeSMF:
			taiFilter = bson.M{
				"smfinfo.tailist": bson.M{
					mongoOpElemMatch: taiBsonM,
				},
			}
		case nfTypeAMF:
			taiFilter = bson.M{
				"amfinfo.tailist": bson.M{
					mongoOpElemMatch: taiBsonM,
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), taiFilter)
	}
}

func handleAmfRegionID(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-15] amf-region-id
	if queryParameters[queryParamAmfRegionID] != nil {
		if targetNfType == nfTypeAMF {
			amfRegionId := queryParameters[queryParamAmfRegionID][0]
			amfRegionIdFilter := bson.M{
				"amfinfo.amfregionid": amfRegionId,
			}
			filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), amfRegionIdFilter)
		}
	}
}

func handleAmfSetID(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-16] amf-set-id
	if queryParameters[queryParamAmfSetID] != nil {
		if targetNfType == nfTypeAMF {
			amfSetId := queryParameters[queryParamAmfSetID][0]
			amfSetIdFilter := bson.M{
				"amfinfo.amfsetid": amfSetId,
			}
			filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), amfSetIdFilter)
		}
	}
}

func handleGuami(queryParameters url.Values, filter bson.M, targetNfType string) {
	// Query-17: guami
	// TODO: NOTE[1]
	if queryParameters["guami"] != nil {
		if targetNfType == nfTypeAMF {
			guami := queryParameters["guami"][0]

			guamiStruct := models.NewGuamiWithDefaults()
			err := json.Unmarshal([]byte(guami), guamiStruct)
			if err != nil {
				logger.DiscoveryLog.Warnln("unmarshal error in guamiStruct:", err)
			}

			guamiByteArray, err := bson.Marshal(guamiStruct)
			if err != nil {
				logger.DiscoveryLog.Warnln(errUnmarshalGuamiByteArray, err)
			}

			guamiBsonM := bson.M{}
			err = bson.Unmarshal(guamiByteArray, &guamiBsonM)
			if err != nil {
				logger.DiscoveryLog.Warnln(errUnmarshalGuamiByteArray, err)
			}

			guamiFilter := bson.M{
				"amfinfo.guamilist": bson.M{
					mongoOpElemMatch: guamiBsonM,
				},
			}

			filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), guamiFilter)
		}
	}
}

func handleSupi(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-18] supi
	var supi string
	if queryParameters["supi"] != nil {
		var supiFilter bson.M
		supi = queryParameters["supi"][0]
		supi = supi[5:]
		switch targetNfType {
		case nfTypePCF:
			supiFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						fieldPcfInfoSupiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: supi,
								},
								fieldEnd: bson.M{
									mongoOpGte: supi,
								},
							},
						},
					},
					{
						fieldPcfInfoSupiRanges: nil,
					},
					{
						fieldPcfInfoSupiRanges: bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		case nfTypeCHF:
			supiFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						"chfinfo.supirangelist": bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: supi,
								},
								fieldEnd: bson.M{
									mongoOpGte: supi,
								},
							},
						},
					},
					{
						"chfinfo.supirangelist": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		case nfTypeAUSF:
			supiFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						"ausfinfo.supiranges": bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: supi,
								},
								fieldEnd: bson.M{
									mongoOpGte: supi,
								},
							},
						},
					},
					{
						"ausfinfo.supiranges": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		case nfTypeUDM:
			supiFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						fieldUdmInfoSupiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: supi,
								},
								fieldEnd: bson.M{
									mongoOpGte: supi,
								},
							},
						},
					},
					{
						fieldUdmInfoSupiRanges: bson.M{
							mongoOpExists: false,
						},

						fieldUdmInfoGpsiRanges: bson.M{
							mongoOpExists: false,
						},

						fieldUdmExtGrpIDRanges: bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		case nfTypeUDR:
			supiFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						fieldUdrInfoSupiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: supi,
								},
								fieldEnd: bson.M{
									mongoOpGte: supi,
								},
							},
						},
					},
					{
						fieldUdrInfoSupiRanges: bson.M{
							mongoOpExists: false,
						},

						fieldUdrInfoGpsiRanges: bson.M{
							mongoOpExists: false,
						},

						fieldUdrExtGroupIDRanges: bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), supiFilter)
	}
}

func handleUeIpv4(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-19] ue-ipv4-address
	if queryParameters[queryParamUeIpv4Address] != nil {
		var ueIpv4AddressFilter bson.M
		if targetNfType == nfTypeBSF {
			ueIpv4Address := queryParameters[queryParamUeIpv4Address][0]
			ueIpv4AddressNumber := context.Ipv4ToInt(ueIpv4Address)
			ueIpv4AddressFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						"bsfinfo.ipv4addressranges": bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: strconv.Itoa(int(ueIpv4AddressNumber)),
								},
								fieldEnd: bson.M{
									mongoOpGte: strconv.Itoa(int(ueIpv4AddressNumber)),
								},
							},
						},
					},
					{
						"bsfinfo.ipv4addressranges": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), ueIpv4AddressFilter)
	}
}

func handleIpDomain(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-20] ip-domain
	if queryParameters[queryParamIpDomain] != nil {
		var ipDomainFilter bson.M
		if targetNfType == nfTypeBSF {
			ipDomain := queryParameters[queryParamIpDomain][0]
			ipDomainFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						"bsfinfo.ipdomainlist": ipDomain,
					},
					{
						"bsfinfo.ipdomainlist": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), ipDomainFilter)
	}
}

func handleUeIpv6Prefix(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-21] ue-ipv6-prefix
	if queryParameters[queryParamUeIpv6Prefix] != nil {
		var ueIpv6PrefixFilter bson.M
		if targetNfType == nfTypeBSF {
			ueIpv6Prefix := queryParameters[queryParamUeIpv6Prefix][0]
			ueIpv6PrefixNumber := context.Ipv6ToInt(ueIpv6Prefix)
			ueIpv6PrefixFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						"bsfinfo.ipv6prefixranges": bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: ueIpv6PrefixNumber.String(),
								},
								fieldEnd: bson.M{
									mongoOpGte: ueIpv6PrefixNumber.String(),
								},
							},
						},
					},
					{
						"bsfinfo.ipv6prefixranges": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), ueIpv6PrefixFilter)
	}
}

func handlePgwInd(queryParameters url.Values, filter bson.M) {
	// [Query-22] pgw-ind
	if queryParameters[queryParamPgwInd] != nil {
		pgwInd := queryParameters[queryParamPgwInd][0]
		if pgwInd == "true" {
			pgwIndFilter := bson.M{
				"smfinfo.pgwfqdn": bson.M{
					mongoOpExists: true,
				},
			}
			filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), pgwIndFilter)
		}
	}
}

func handlePgw(queryParameters url.Values, filter bson.M) {
	// [Query-23] pgw
	if queryParameters["pgw"] != nil {
		pgw := queryParameters["pgw"][0]
		pgwFilter := bson.M{
			"smfinfo.pgwfqdn": pgw,
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), pgwFilter)
	}
}

func handleGpsi(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-24] gpsi
	if queryParameters["gpsi"] != nil {
		var gpsiFilter bson.M
		gpsi := queryParameters["gpsi"][0]
		gpsi = gpsi[7:]
		switch targetNfType {
		case nfTypeCHF:
			gpsiFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						"chfinfo.gpsirangelist": bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: gpsi,
								},
								fieldEnd: bson.M{
									mongoOpGte: gpsi,
								},
							},
						},
					},
					{
						"chfinfo.gpsirangelist": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		case nfTypeUDM:
			gpsiFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						fieldUdmInfoGpsiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: gpsi,
								},
								fieldEnd: bson.M{
									mongoOpGte: gpsi,
								},
							},
						},
					},
					{
						fieldUdmInfoSupiRanges: bson.M{
							mongoOpExists: false,
						},

						fieldUdmInfoGpsiRanges: bson.M{
							mongoOpExists: false,
						},

						fieldUdmExtGrpIDRanges: bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		case nfTypeUDR:
			gpsiFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						fieldUdrInfoGpsiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: gpsi,
								},
								fieldEnd: bson.M{
									mongoOpGte: gpsi,
								},
							},
						},
					},
					{
						fieldUdrInfoSupiRanges: bson.M{
							mongoOpExists: false,
						},

						fieldUdrInfoGpsiRanges: bson.M{
							mongoOpExists: false,
						},

						fieldUdrExtGroupIDRanges: bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), gpsiFilter)
	}
}

func handleExternalGroupIdentity(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-25] external-group-identity
	if queryParameters[queryParamExternalGroupIdentity] != nil {
		var externalGroupIdentityFilter bson.M
		externalGroupIdentity := queryParameters[queryParamExternalGroupIdentity][0]

		encodedGroupId := context.EncodeGroupId(externalGroupIdentity)
		switch targetNfType {
		case nfTypeUDM:
			externalGroupIdentityFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						fieldUdmExtGrpIDRanges: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: encodedGroupId,
								},
								fieldEnd: bson.M{
									mongoOpGte: encodedGroupId,
								},
							},
						},
					},
					{
						fieldUdmInfoSupiRanges: bson.M{
							mongoOpExists: false,
						},

						fieldUdmInfoGpsiRanges: bson.M{
							mongoOpExists: false,
						},

						fieldUdmExtGrpIDRanges: bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		case nfTypeUDR:
			externalGroupIdentityFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						fieldUdrExtGroupIDRanges: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: encodedGroupId,
								},
								fieldEnd: bson.M{
									mongoOpGte: encodedGroupId,
								},
							},
						},
					},
					{
						fieldUdrInfoSupiRanges: bson.M{
							mongoOpExists: false,
						},

						fieldUdrInfoGpsiRanges: bson.M{
							mongoOpExists: false,
						},

						fieldUdrExtGroupIDRanges: bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), externalGroupIdentityFilter)
	}
}

func handleDataSet(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-26] data-set
	if queryParameters[queryParamDataSet] != nil {
		var dataSetFilter bson.M
		dataSet := queryParameters[queryParamDataSet]
		if targetNfType == nfTypeUDR {
			dataSetFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						"udrinfo.supporteddatasets": dataSet,
					},
					{
						"udrinfo.supporteddatasets": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), dataSetFilter)
	}
}

func handleRoutingIndicator(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-27] routing-indicator
	if queryParameters[queryParamRoutingIndicator] != nil {
		var routingIndicatorFilter bson.M
		routingIndicator := queryParameters[queryParamRoutingIndicator][0]
		switch targetNfType {
		case nfTypeAUSF:
			routingIndicatorFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						"ausfinfo.routingindicators": routingIndicator,
					},
					{
						"ausfinfo.routingindicators": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		case nfTypeUDM:
			routingIndicatorFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						"udminfo.routingindicators": routingIndicator,
					},
					{
						"udminfo.routingindicators": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), routingIndicatorFilter)
	}
}

func handleGroupIDList(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-28] group-id-list
	if queryParameters[queryParamGroupIDList] != nil {
		var groupIdListFilter bson.M

		groupIdList := queryParameters[queryParamGroupIDList][0]
		groupIdListSplit := strings.Split(groupIdList, ",")
		var groupIdListBsonArray bson.A

		for _, v := range groupIdListSplit {
			groupIdListBsonArray = append(groupIdListBsonArray, v)
		}
		switch targetNfType {
		case nfTypeUDR:
			groupIdListFilter = bson.M{
				"udrinfo.groupid": bson.M{
					mongoOpIn: groupIdListBsonArray,
				},
			}
		case nfTypeUDM:
			groupIdListFilter = bson.M{
				"udminfo.groupid": bson.M{
					mongoOpIn: groupIdListBsonArray,
				},
			}
		case nfTypeAUSF:
			groupIdListFilter = bson.M{
				"ausfinfo.groupid": bson.M{
					mongoOpIn: groupIdListBsonArray,
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), groupIdListFilter)
	}
}

func handleDnaiList(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-29] dnai-list
	if queryParameters[queryParamDnaiList] != nil {
		var dnaiFilter bson.M
		dnaiList := queryParameters[queryParamDnaiList][0]
		dnaiListSplit := strings.Split(dnaiList, ",")
		var dnaiListBsonArray bson.A

		for _, v := range dnaiListSplit {
			dnaiListBsonArray = append(dnaiListBsonArray, v)
		}
		if targetNfType == nfTypeUPF {
			dnaiFilter = bson.M{
				"upfinfo.snssaiupfinfolist": bson.M{
					mongoOpElemMatch: bson.M{
						fieldDnnUpfInfoList: bson.M{
							mongoOpElemMatch: bson.M{
								"dnailist": bson.M{
									mongoOpIn: dnaiListBsonArray,
								},
							},
						},
					},
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), dnaiFilter)
	}
}

func handleUpfIwkEpsInd(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-30] upf-iwk-eps-ind
	if queryParameters[queryParamUpfIwkEpsInd] != nil {
		var upfIwkEpsIndFilter bson.M
		// upfIwkEpsInd := queryParameters["upf-iwk-eps-ind"][0]
		if targetNfType == nfTypeUPF {
			upfIwkEpsIndFilter = bson.M{
				"upfinfo.iwkepsind": true,
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), upfIwkEpsIndFilter)
	}
}

func handleChfSupportedPlmn(queryParameters url.Values, filter bson.M, targetNfType string) {
	// [Query-31] chf-supported-plmn
	if queryParameters[queryParamChfSupportedPlmn] != nil {
		var chfSupportedPlmnFilter bson.M
		chfSupportedPlmn := queryParameters[queryParamChfSupportedPlmn][0]
		chfSupportedPlmnStruct := models.NewPlmnIdWithDefaults()
		err := json.Unmarshal([]byte(chfSupportedPlmn), chfSupportedPlmnStruct)
		if err != nil {
			logger.DiscoveryLog.Warnln("unmarshal error in chfSupportedPlmnStruct:", err)
		}

		encodedchfSupportedPlmn := chfSupportedPlmnStruct.Mcc + chfSupportedPlmnStruct.Mnc

		if targetNfType == nfTypeCHF {
			chfSupportedPlmnFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						fieldChfInfoPlmnRangeList: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: encodedchfSupportedPlmn,
								},
								fieldEnd: bson.M{
									mongoOpGte: encodedchfSupportedPlmn,
								},
							},
						},
					},
					{
						fieldChfInfoPlmnRangeList: bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), chfSupportedPlmnFilter)
	}
}

func handlePreferredLocality(queryParameters url.Values, filter bson.M) {
	// [Query-32]  preferred-locality
	// TODO: if no match
	if queryParameters[queryParamPreferredLocality] != nil {
		preferredLocality := queryParameters[queryParamPreferredLocality][0]
		preferredLocalityFilter := bson.M{
			"locality": preferredLocality,
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), preferredLocalityFilter)
	}
}

func handleAccessType(queryParameters url.Values, filter bson.M) {
	// [Query-33] access-type
	if queryParameters[queryParamAccessType] != nil {
		accessType := queryParameters[queryParamAccessType][0]
		accessTypeFilter := bson.M{
			mongoOpOr: []bson.M{
				{
					"smfinfo.accesstype": accessType,
				},
				{
					"smfinfo.accesstype": bson.M{
						mongoOpExists: false,
					},
				},
			},
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), accessTypeFilter)
	}
}

func handleSupportedFeatures(queryParameters url.Values, filter bson.M) {
	// [Query-34] supported-features
	if queryParameters[queryParamSupportedFeatures] != nil {
		supportedFeatures := queryParameters[queryParamSupportedFeatures][0]
		supportedFeaturesFilter := bson.M{
			fieldNfServices: bson.M{
				mongoOpElemMatch: bson.M{
					"supportedfeatures": supportedFeatures,
				},
			},
		}
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), supportedFeaturesFilter)
	}
}

func handleComplexQuery(queryParameters url.Values, filter bson.M) {
	// [Query-35] complexQuery
	if queryParameters["complexQuery"] != nil {
		// translate raw data to complexQuery structure
		complexQuery := queryParameters["complexQuery"][0]
		complexQueryStruct := &models.ComplexQuery{}
		err := json.Unmarshal([]byte(complexQuery), complexQueryStruct)
		if err != nil {
			logger.DiscoveryLog.Warnln("unmarshal error in complexQuery:", err)
		}
		complexQueryFilter := complexQueryFilter(complexQueryStruct)
		filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), complexQueryFilter)
	}
}

const (
	COMPLEX_QUERY_TYPE_CNF string = "CNF"
	COMPLEX_QUERY_TYPE_DNF string = "DNF"
)

type AtomElem struct {
	value    string
	negative bool
}

func complexQueryFilter(complexQueryParameter *models.ComplexQuery) bson.M {
	complexQueryType := ""
	if complexQueryParameter.Cnf != nil {
		complexQueryType = COMPLEX_QUERY_TYPE_CNF
	} else {
		complexQueryType = COMPLEX_QUERY_TYPE_DNF
	}

	// build the filter
	var filter bson.M

	if complexQueryType == COMPLEX_QUERY_TYPE_CNF {
		filter = bson.M{
			mongoOpAnd: []bson.M{},
		}
		for _, cnfUnit := range complexQueryParameter.Cnf.GetCnfUnits() {
			queryParameters := make(map[string]*AtomElem)
			var cnfUnitFilter bson.M
			for _, atom := range cnfUnit.CnfUnit {
				strValue, ok := atom.Value.(string)
				if !ok {
					logger.AppLog.Errorln("the value is not a string")
					continue
				}
				queryParameters[atom.Attr] = &AtomElem{value: strValue, negative: atom.GetNegative()}
			}
			cnfUnitFilter = complexQueryFilterSubprocess(queryParameters, complexQueryType)

			filter[mongoOpAnd] = append(filter[mongoOpAnd].([]bson.M), cnfUnitFilter)
		}
	} else {
		filter = bson.M{
			mongoOpOr: []bson.M{},
		}
	}
	return filter
}

func complexQueryFilterSubprocess(queryParameters map[string]*AtomElem, complexQueryType string) bson.M {
	var filter bson.M
	var logicalOperator string

	switch complexQueryType {
	case COMPLEX_QUERY_TYPE_CNF:
		logicalOperator = mongoOpOr
	case COMPLEX_QUERY_TYPE_DNF:
		logicalOperator = mongoOpAnd
	}

	filter = bson.M{
		logicalOperator: []bson.M{},
	}
	targetNfType := addTargetNfTypeFilter(queryParameters, filter, logicalOperator)
	addServiceNamesFilter(queryParameters, filter, logicalOperator)
	addRequesterNfInstanceFqdnFilter(queryParameters, filter, logicalOperator)
	addTargetPlmnListFilter(queryParameters, filter, logicalOperator)
	addTargetNfInstanceIDFilter(queryParameters, filter, logicalOperator)
	addTargetNfFqdnFilter(queryParameters, filter, logicalOperator)
	addSnssaisFilter(queryParameters, filter, logicalOperator)
	addNsiListFilter(queryParameters, filter, logicalOperator)
	addDnnFilter(queryParameters, filter, logicalOperator, targetNfType)
	addSmfServingAreaFilter(queryParameters, filter, logicalOperator, targetNfType)
	addTaiFilter(queryParameters, filter, logicalOperator, targetNfType)
	addAmfRegionFilter(queryParameters, filter, logicalOperator, targetNfType)
	addAmfSetIdFilter(queryParameters, filter, logicalOperator, targetNfType)
	addGuamiFilter(queryParameters, filter, logicalOperator, targetNfType)
	addSupiFilter(queryParameters, filter, logicalOperator, targetNfType)
	addIpv4Filter(queryParameters, filter, logicalOperator, targetNfType)
	addIpDomainFilter(queryParameters, filter, logicalOperator, targetNfType)
	addIpv6PrefixFilter(queryParameters, filter, logicalOperator, targetNfType)
	addPgwIndFilter(queryParameters, filter, logicalOperator)
	addPgwFilter(queryParameters, filter, logicalOperator)
	addGpsiFilter(queryParameters, filter, logicalOperator, targetNfType)
	addExternalGroupFilter(queryParameters, filter, logicalOperator, targetNfType)
	addDataSetFilter(queryParameters, filter, logicalOperator, targetNfType)
	addRoutingIndicatorFilter(queryParameters, filter, logicalOperator, targetNfType)
	addGroupIdListFilter(queryParameters, filter, logicalOperator, targetNfType)
	addDnaiFilter(queryParameters, filter, logicalOperator, targetNfType)
	addUpfIwkEpsFilter(queryParameters, filter, logicalOperator, targetNfType)
	addChfSupportedPlmnFilter(queryParameters, filter, logicalOperator, targetNfType)
	addPreferredLocalityFilter(queryParameters, filter, logicalOperator)
	addAccessTypeFilter(queryParameters, filter, logicalOperator)
	addSupportedFeaturesFilter(queryParameters, filter, logicalOperator)

	return filter
}

func addTargetNfTypeFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) string {
	// [Query-1] target-nf-type
	var targetNfType string
	if targetNfTypeParam, ok := queryParameters[queryParamTargetNFType]; ok && targetNfTypeParam != nil {
		var targetNfTypeFilter bson.M
		targetNfType = targetNfTypeParam.value
		negative := targetNfTypeParam.negative
		if negative {
			targetNfTypeFilter = bson.M{
				fieldNfTypeLower: bson.M{
					mongoOpNe: targetNfType,
				},
			}
		} else if !negative {
			targetNfTypeFilter = bson.M{
				fieldNfTypeLower: targetNfType,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), targetNfTypeFilter)
	}
	return targetNfType
}

// [Query-2] requester-nf-type
// requesterNfType := queryParameters["requester-nf-type"].value
// TODO
func addServiceNamesFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) {
	// [Query-3] service-names
	// TODO: return exist service name
	if queryParameters[queryParamServiceNames] != nil {
		var serviceNamesFilter bson.M
		serviceNames := queryParameters[queryParamServiceNames].value
		serviceNamesSplit := strings.Split(serviceNames, ",")
		var serviceNamesBsonArray bson.A

		for _, v := range serviceNamesSplit {
			serviceNamesBsonArray = append(serviceNamesBsonArray, v)
		}

		negative := queryParameters[queryParamServiceNames].negative
		if negative {
			serviceNamesFilter = bson.M{
				fieldNfServices: bson.M{
					mongoOpElemMatch: bson.M{
						fieldServiceName: bson.M{
							// get all service in array
							"$nin": serviceNamesBsonArray,
						},
						// the service need to be registered
						fieldNfServiceStatus: nfServiceStatusRegistered,
					},
				},
			}
		} else if !negative {
			serviceNamesFilter = bson.M{
				fieldNfServices: bson.M{
					mongoOpElemMatch: bson.M{
						fieldServiceName: bson.M{
							// get all service in array
							mongoOpIn: serviceNamesBsonArray,
						},
						// the service need to be registered
						fieldNfServiceStatus: nfServiceStatusRegistered,
					},
				},
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), serviceNamesFilter)
	}
}

func addRequesterNfInstanceFqdnFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) {
	// [Query-4] requester-nfinstance-fqdn
	if queryParameters[queryParamRequesterNfInstanceFqdn] != nil {
		var requesterNfinstanceFqdnFilter bson.M
		requesterNfinstanceFqdn := queryParameters[queryParamRequesterNfInstanceFqdn].value

		negative := queryParameters[queryParamRequesterNfInstanceFqdn].negative
		if negative {
			requesterNfinstanceFqdnFilter = bson.M{
				fieldNfServices: bson.M{
					mongoOpElemMatch: bson.M{
						fieldAllowedNfDomains: requesterNfinstanceFqdn,
					},
				},
			}
		} else if !negative {
			requesterNfinstanceFqdnFilter = bson.M{
				fieldNfServices: bson.M{
					mongoOpElemMatch: bson.M{
						fieldAllowedNfDomains: bson.M{
							mongoOpNe: requesterNfinstanceFqdn,
						},
					},
				},
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), requesterNfinstanceFqdnFilter)
	}
}

func addTargetPlmnListFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) {
	// [Query-5] target-plmn-list [C] = Mcc + Mnc
	// Mcc: Pattern: '^[0-9]{3}$'
	// Mnc: Pattern: '^[0-9]{2,3}$'
	if queryParameters[queryParamTargetPlmnList] != nil {
		targetPlmnList := queryParameters[queryParamTargetPlmnList].value
		targetPlmnListSplit := strings.Split(targetPlmnList, ",")
		var targetPlmnListBsonArray bson.A

		var temptargetPlmn string
		for i, v := range targetPlmnListSplit {
			if i%2 == 0 {
				temptargetPlmn = v
			} else {
				temptargetPlmn += ","
				temptargetPlmn += v

				targetPlmnListtruct := models.NewPlmnIdWithDefaults()
				err := json.Unmarshal([]byte(temptargetPlmn), targetPlmnListtruct)
				if err != nil {
					logger.DiscoveryLog.Warnln("unmarshal error in targetPlmnListstruct:", err)
				}

				targetPlmnByteArray, err := bson.Marshal(targetPlmnListtruct)
				if err != nil {
					logger.DiscoveryLog.Warnln("marshal error in targetPlmnListtruct:", err)
				}

				targetPlmnBsonM := bson.M{}
				err = bson.Unmarshal(targetPlmnByteArray, &targetPlmnBsonM)
				if err != nil {
					logger.DiscoveryLog.Warnln("unmarshal error in targetPlmnBsonM:", err)
				}

				targetPlmnListBsonArray = append(targetPlmnListBsonArray, targetPlmnBsonM)
			}
		}

		var targetPlmnListFilter bson.M
		negative := queryParameters[queryParamTargetPlmnList].negative
		if negative {
			targetPlmnListFilter = bson.M{
				fieldPlmnList: bson.M{
					"$nin": targetPlmnListBsonArray,
				},
			}
		} else if !negative {
			targetPlmnListFilter = bson.M{
				fieldPlmnList: bson.M{
					mongoOpIn: targetPlmnListBsonArray,
				},
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), targetPlmnListFilter)
	}
}

// [Query-6] requester-plmn-list
// if queryParameters["requester-plmn-list"] != nil {
// requesterPlmnPist := queryParameters["requester-plmn-list"].value
// TODO
// }
func addTargetNfInstanceIDFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) {
	// [Query-7] target-nf-instanceid
	if queryParameters[queryParamTargetNfInstanceID] != nil {
		targetNfInstanceid := queryParameters[queryParamTargetNfInstanceID].value
		var nfInstanceIdFilter bson.M

		negative := queryParameters[queryParamTargetNfInstanceID].negative
		if negative {
			nfInstanceIdFilter = bson.M{
				fieldNfInstanceId: bson.M{
					mongoOpNe: targetNfInstanceid,
				},
			}
		} else if !negative {
			nfInstanceIdFilter = bson.M{
				fieldNfInstanceId: targetNfInstanceid,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), nfInstanceIdFilter)
	}
}

func addTargetNfFqdnFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) {
	// [Query-8] target-nf-fqdn
	if queryParameters[queryParamTargetNfFqdn] != nil {
		targetNfFqdn := queryParameters[queryParamTargetNfFqdn].value
		fqdnFilter := bson.M{
			fieldFqdn: targetNfFqdn,
		}
		if queryParameters[queryParamTargetNfFqdn].negative {
			fqdnFilter = bson.M{
				fieldFqdn: bson.M{
					mongoOpNe: targetNfFqdn,
				},
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), fqdnFilter)
	}
}

// [Query-9] hnrf-uri
// for Roaming
func addSnssaisFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) {
	// [Query-10] snssais
	// Pattern: '^[A-Fa-f0-9]{6}$'
	if queryParameters[fieldSnssais] != nil {
		snssaisFilters := buildSnssaisElemMatchFilters(queryParameters[fieldSnssais].value)
		if len(snssaisFilters) > 0 {
			var snssaisFilter bson.M
			switch len(snssaisFilters) {
			case 1:
				snssaisFilter = snssaisFilters[0]
			default:
				snssaisFilter = bson.M{
					mongoOpOr: snssaisFilters,
				}
			}
			if queryParameters[fieldSnssais].negative {
				snssaisFilter = bson.M{
					"$nor": snssaisFilters,
				}
			}
			filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), snssaisFilter)
		}
	}
}

func addNsiListFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) {
	// [Query-11] nsi-list
	if queryParameters[queryParamNsiList] != nil {
		nsiList := queryParameters[queryParamNsiList].value
		nsiListSplit := strings.Split(nsiList, ",")
		var nsiListBsonArray bson.A
		for _, v := range nsiListSplit {
			nsiListBsonArray = append(nsiListBsonArray, v)
		}
		nsiListFilter := bson.M{
			"nsilist": bson.M{
				"$all": nsiListBsonArray,
			},
		}
		if queryParameters[queryParamNsiList].negative {
			nsiListFilter = bson.M{
				mongoOpNot: nsiListFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), nsiListFilter)
	}
}

func addDnnFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-12] dnn
	if queryParameters[queryParamDnn] != nil {
		dnn := queryParameters[queryParamDnn].value
		var dnnFilter bson.M
		switch targetNfType {
		case nfTypeSMF:
			dnnFilter = bson.M{
				"smfinfo.snssaismfinfolist": bson.M{
					mongoOpElemMatch: bson.M{
						"dnnsmfinfolist": bson.M{
							mongoOpElemMatch: bson.M{
								mongoOpOr: []bson.M{
									{queryParamDnn: dnn},
									{"dnn.string": dnn},
								},
							},
						},
					},
				},
			}
		case nfTypeUPF:
			dnnFilter = bson.M{
				fieldUpfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"snssaiupfinfolist": bson.M{
							mongoOpElemMatch: bson.M{
								fieldDnnUpfInfoList: bson.M{
									mongoOpElemMatch: bson.M{
										queryParamDnn: dnn,
									},
								},
							},
						},
					},
				},
			}
		case nfTypeBSF:
			dnnFilter = bson.M{
				fieldBsfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"dnnlist": dnn,
					},
				},
			}
		}
		if queryParameters[queryParamDnn].negative {
			dnnFilter = bson.M{
				mongoOpNot: dnnFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), dnnFilter)
	}
}

func addSmfServingAreaFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-13] smf-serving-area
	if queryParameters[queryParamSmfServingArea] != nil {
		var smfServingAreaFilter bson.M
		smfServingArea := queryParameters[queryParamSmfServingArea].value
		if targetNfType == nfTypeUPF {
			smfServingAreaFilter = bson.M{
				fieldUpfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"smfservingarea": smfServingArea,
					},
				},
			}
		}
		if queryParameters[queryParamSmfServingArea].negative {
			smfServingAreaFilter = bson.M{
				mongoOpNot: smfServingAreaFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), smfServingAreaFilter)
	}
}

func addTaiFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-14] tai
	if queryParameters["tai"] != nil {
		var taiFilter bson.M
		tai := queryParameters["tai"].value
		taiSplit := strings.Split(tai, ",")
		tempTai := taiSplit[0] + "," + taiSplit[1]

		taiStruct := models.NewTaiWithDefaults()
		err := json.Unmarshal([]byte(tempTai), taiStruct)
		if err != nil {
			logger.DiscoveryLog.Warnln("unmarshal error in taiStruct:", err)
		}

		taiByteArray, err := bson.Marshal(taiStruct)
		if err != nil {
			logger.DiscoveryLog.Warnln(errUnmarshalTaiByteArray, err)
		}

		taiBsonM := bson.M{}
		err = bson.Unmarshal(taiByteArray, &taiBsonM)
		if err != nil {
			logger.DiscoveryLog.Warnln(errUnmarshalTaiByteArray, err)
		}
		switch targetNfType {
		case nfTypeSMF:
			taiFilter = bson.M{
				fieldSmfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"tailist": taiBsonM,
					},
				},
			}
		case nfTypeAMF:
			taiFilter = bson.M{
				fieldAmfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"tailist": taiBsonM,
					},
				},
			}
		}
		if queryParameters["tai"].negative {
			taiFilter = bson.M{
				mongoOpNot: taiFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), taiFilter)
	}
}

func addAmfRegionFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-15] amf-region-id
	if queryParameters[queryParamAmfRegionID] != nil {
		var amfRegionIdFilter bson.M
		if targetNfType == nfTypeAMF {
			amfRegionId := queryParameters[queryParamAmfRegionID].value
			amfRegionIdFilter = bson.M{
				fieldAmfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"amfregionid": amfRegionId,
					},
				},
			}
		}
		if queryParameters[queryParamAmfRegionID].negative {
			amfRegionIdFilter = bson.M{
				mongoOpNot: amfRegionIdFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), amfRegionIdFilter)
	}
}

func addAmfSetIdFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-16] amf-set-id
	if queryParameters[queryParamAmfSetID] != nil {
		var amfSetIdFilter bson.M
		if targetNfType == nfTypeAMF {
			amfSetId := queryParameters[queryParamAmfSetID].value
			amfSetIdFilter = bson.M{
				fieldAmfInfo: bson.M{
					mongoOpElemMatch: bson.M{ // TOCHECK : elemMatch
						"amfsetid": amfSetId,
					},
				},
			}
		}
		if queryParameters[queryParamAmfSetID].negative {
			amfSetIdFilter = bson.M{
				mongoOpNot: amfSetIdFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), amfSetIdFilter)
	}
}

func addGuamiFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// Query-17: guami
	// TODO: NOTE[1]
	if queryParameters["guami"] != nil {
		var guamiFilter bson.M
		if targetNfType == nfTypeAMF {
			guami := queryParameters["guami"].value
			guamiSplit := strings.Split(guami, ",")
			tempguami := guamiSplit[0] + "," + guamiSplit[1]

			guamiStruct := models.NewGuamiWithDefaults()
			err := json.Unmarshal([]byte(tempguami), guamiStruct)
			if err != nil {
				logger.DiscoveryLog.Warnln("unmarshal error in guamiStruct:", err)
			}

			guamiByteArray, err := bson.Marshal(guamiStruct)
			if err != nil {
				logger.DiscoveryLog.Warnln(errUnmarshalGuamiByteArray, err)
			}

			guamiBsonM := bson.M{}
			err = bson.Unmarshal(guamiByteArray, &guamiBsonM)
			if err != nil {
				logger.DiscoveryLog.Warnln(errUnmarshalGuamiByteArray, err)
			}

			guamiFilter = bson.M{
				fieldAmfInfo: bson.M{
					mongoOpElemMatch: bson.M{ // TOCHECK : elemMatch
						"guamilist": bson.M{
							mongoOpElemMatch: guamiBsonM,
						},
					},
				},
			}
		}
		if queryParameters["guami"].negative {
			guamiFilter = bson.M{
				mongoOpNot: guamiFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), guamiFilter)
	}
}

func addSupiFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-18] supi
	var supi string
	if queryParameters["supi"] != nil {
		var supiFilter bson.M
		supi = queryParameters["supi"].value
		switch targetNfType {
		case nfTypePCF:
			supiFilter = bson.M{
				"pcfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						fieldSupiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: supi[0],
								},
								fieldEnd: bson.M{
									mongoOpGte: supi[0],
								},
							},
						},
					},
				},
			}
		case nfTypeCHF:
			supiFilter = bson.M{
				fieldChfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						fieldSupiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: supi[0],
								},
								fieldEnd: bson.M{
									mongoOpGte: supi[0],
								},
							},
						},
					},
				},
			}
		case nfTypeAUSF:
			supiFilter = bson.M{
				fieldAusfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						fieldSupiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: supi[0],
								},
								fieldEnd: bson.M{
									mongoOpGte: supi[0],
								},
							},
						},
					},
				},
			}
		case nfTypeUDM:
			supiFilter = bson.M{
				fieldUdmInfo: bson.M{
					mongoOpElemMatch: bson.M{
						fieldSupiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: supi[0],
								},
								fieldEnd: bson.M{
									mongoOpGte: supi[0],
								},
							},
						},
					},
				},
			}
		case nfTypeUDR:
			supiFilter = bson.M{
				fieldUdrInfo: bson.M{
					mongoOpElemMatch: bson.M{
						fieldSupiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: supi[0],
								},
								fieldEnd: bson.M{
									mongoOpGte: supi[0],
								},
							},
						},
					},
				},
			}
		}
		if queryParameters["supi"].negative {
			supiFilter = bson.M{
				mongoOpNot: supiFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), supiFilter)
	}
}

func addIpv4Filter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-19] ue-ipv4-address
	if queryParameters[queryParamUeIpv4Address] != nil {
		var ueIpv4AddressFilter bson.M
		if targetNfType == nfTypeBSF {
			ueIpv4Address := queryParameters[queryParamUeIpv4Address].value
			ueIpv4AddressNumber := context.Ipv4ToInt(ueIpv4Address)
			ueIpv4AddressFilter = bson.M{
				fieldBsfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"ipv4addressranges": bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: ueIpv4AddressNumber,
								},
								fieldEnd: bson.M{
									mongoOpGte: ueIpv4AddressNumber,
								},
							},
						},
					},
				},
			}
		}
		if queryParameters[queryParamUeIpv4Address].negative {
			ueIpv4AddressFilter = bson.M{
				mongoOpNot: ueIpv4AddressFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), ueIpv4AddressFilter)
	}
}

func addIpDomainFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-20] ip-domain
	if queryParameters[queryParamIpDomain] != nil {
		var ipDomainFilter bson.M
		if targetNfType == nfTypeBSF {
			ipDomain := queryParameters[queryParamIpDomain].value
			ipDomainFilter = bson.M{
				fieldBsfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"ipdomainlist": ipDomain,
					},
				},
			}
		}
		if queryParameters[queryParamIpDomain].negative {
			ipDomainFilter = bson.M{
				mongoOpNot: ipDomainFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), ipDomainFilter)
	}
}

func addIpv6PrefixFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-21] ue-ipv6-prefix
	if queryParameters[queryParamUeIpv6Prefix] != nil {
		var ueIpv6PrefixFilter bson.M
		if targetNfType == nfTypeBSF {
			ueIpv6Prefix := queryParameters[queryParamUeIpv6Prefix].value
			ueIpv6PrefixNumber := context.Ipv6ToInt(ueIpv6Prefix)
			ueIpv6PrefixFilter = bson.M{
				fieldBsfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"ipv6prefixranges": bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: ueIpv6PrefixNumber,
								},
								fieldEnd: bson.M{
									mongoOpGte: ueIpv6PrefixNumber,
								},
							},
						},
					},
				},
			}
		}
		if queryParameters[queryParamUeIpv6Prefix].negative {
			ueIpv6PrefixFilter = bson.M{
				mongoOpNot: ueIpv6PrefixFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), ueIpv6PrefixFilter)
	}
}

func addPgwIndFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) {
	// [Query-22] pgw-ind
	if queryParameters[queryParamPgwInd] != nil {
		var pgwIndFilter bson.M
		pgwInd := queryParameters[queryParamPgwInd].value
		if pgwInd == "true" {
			pgwIndFilter = bson.M{
				fieldSmfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"pgwfqdn": bson.M{
							mongoOpNe: "",
						},
					},
				},
			}
		}
		if queryParameters[queryParamPgwInd].negative {
			pgwIndFilter = bson.M{
				mongoOpNot: pgwIndFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), pgwIndFilter)
	}
}

func addPgwFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) {
	// [Query-23] pgw
	if queryParameters["pgw"] != nil {
		pgw := queryParameters["pgw"].value
		pgwFilter := bson.M{
			fieldSmfInfo: bson.M{
				mongoOpElemMatch: bson.M{
					"pgwfqdn": pgw,
				},
			},
		}
		if queryParameters["pgw"].negative {
			pgwFilter = bson.M{
				mongoOpNot: pgwFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), pgwFilter)
	}
}

func addGpsiFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-24] gpsi
	if queryParameters["gpsi"] != nil {
		var gpsiFilter bson.M
		gpsi := queryParameters["gpsi"].value
		switch targetNfType {
		case nfTypeCHF:
			gpsiFilter = bson.M{
				fieldChfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						fieldGpsiRangeList: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: gpsi,
								},
								fieldEnd: bson.M{
									mongoOpGte: gpsi,
								},
							},
						},
					},
				},
			}
		case nfTypeUDM:
			gpsiFilter = bson.M{
				fieldUdmInfo: bson.M{
					mongoOpElemMatch: bson.M{
						fieldGpsiRangeList: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: gpsi[0],
								},
								fieldEnd: bson.M{
									mongoOpGte: gpsi[0],
								},
							},
						},
					},
				},
			}
		case nfTypeUDR:
			gpsiFilter = bson.M{
				fieldUdrInfo: bson.M{
					mongoOpElemMatch: bson.M{
						fieldGpsiRangeList: bson.M{
							mongoOpElemMatch: bson.M{
								fieldStart: bson.M{
									mongoOpLte: gpsi[0],
								},
								fieldEnd: bson.M{
									mongoOpGte: gpsi[0],
								},
							},
						},
					},
				},
			}
		}
		if queryParameters["gpsi"].negative {
			gpsiFilter = bson.M{
				mongoOpNot: gpsiFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), gpsiFilter)
	}
}

func addExternalGroupFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-25] external-group-identity
	if queryParameters[queryParamExternalGroupIdentity] != nil {
		var externalGroupIdentityFilter bson.M
		externalGroupIdentity := queryParameters[queryParamExternalGroupIdentity].value
		switch targetNfType {
		case nfTypeUDM:
			externalGroupIdentityFilter = bson.M{
				fieldUdmInfo: bson.M{
					mongoOpElemMatch: bson.M{
						fieldGroupId: externalGroupIdentity,
					},
				},
			}
		case nfTypeUDR:
			externalGroupIdentityFilter = bson.M{
				fieldUdrInfo: bson.M{
					mongoOpElemMatch: bson.M{
						fieldGroupId: externalGroupIdentity,
					},
				},
			}
		}
		if queryParameters[queryParamExternalGroupIdentity].negative {
			externalGroupIdentityFilter = bson.M{
				mongoOpNot: externalGroupIdentityFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), externalGroupIdentityFilter)
	}
}

func addDataSetFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-26] data-set
	if queryParameters[queryParamDataSet] != nil {
		var dataSetFilter bson.M
		dataSet := queryParameters[queryParamDataSet]
		if targetNfType == nfTypeUDR {
			dataSetFilter = bson.M{
				fieldUdrInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"supporteddatasets": dataSet,
					},
				},
			}
		}
		if queryParameters[queryParamDataSet].negative {
			dataSetFilter = bson.M{
				mongoOpNot: dataSetFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), dataSetFilter)
	}
}

func addRoutingIndicatorFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-27] routing-indicator
	if queryParameters[queryParamRoutingIndicator] != nil {
		var routingIndicatorFilter bson.M
		routingIndicator := queryParameters[queryParamRoutingIndicator].value
		switch targetNfType {
		case nfTypeAUSF:
			routingIndicatorFilter = bson.M{
				fieldAusfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"routingindicators": routingIndicator,
					},
				},
			}
		case nfTypeUDM:
			routingIndicatorFilter = bson.M{
				fieldUdmInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"routingindicators": routingIndicator,
					},
				},
			}
		}
		if queryParameters[queryParamRoutingIndicator].negative {
			routingIndicatorFilter = bson.M{
				mongoOpNot: routingIndicatorFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), routingIndicatorFilter)
	}
}

func addGroupIdListFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-28] group-id-list
	if queryParameters[queryParamGroupIDList] != nil {
		var groupIdListFilter bson.M

		groupIdList := queryParameters[queryParamGroupIDList].value
		groupIdListSplit := strings.Split(groupIdList, ",")
		var groupIdListBsonArray bson.A

		for _, v := range groupIdListSplit {
			groupIdListBsonArray = append(groupIdListBsonArray, v)
		}
		switch targetNfType {
		case nfTypeUDR:
			groupIdListFilter = bson.M{
				fieldUdrInfo: bson.M{
					mongoOpElemMatch: bson.M{
						fieldGroupId: bson.M{
							mongoOpIn: groupIdListBsonArray,
						},
					},
				},
			}
		case nfTypeUDM:
			groupIdListFilter = bson.M{
				fieldUdmInfo: bson.M{
					mongoOpElemMatch: bson.M{
						fieldGroupId: bson.M{
							mongoOpIn: groupIdListBsonArray,
						},
					},
				},
			}
		case nfTypeAUSF:
			groupIdListFilter = bson.M{
				fieldAusfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						fieldGroupId: bson.M{
							mongoOpIn: groupIdListBsonArray,
						},
					},
				},
			}
		}
		if queryParameters[queryParamGroupIDList].negative {
			groupIdListFilter = bson.M{
				mongoOpNot: groupIdListFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), groupIdListFilter)
	}
}

func addDnaiFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-29] dnai-list
	if queryParameters[queryParamDnaiList] != nil {
		var dnaiFilter bson.M
		dnaiList := queryParameters[queryParamDnaiList].value
		dnaiListSplit := strings.Split(dnaiList, ",")
		var dnaiListBsonArray bson.A

		for _, v := range dnaiListSplit {
			dnaiListBsonArray = append(dnaiListBsonArray, v)
		}
		if targetNfType == nfTypeUPF {
			dnaiFilter = bson.M{
				fieldUpfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"snssaiupfinfolist": bson.M{
							mongoOpElemMatch: bson.M{
								fieldDnnUpfInfoList: bson.M{
									mongoOpElemMatch: bson.M{
										"dnailist": dnaiListBsonArray,
									},
								},
							},
						},
					},
				},
			}
		}
		if queryParameters[queryParamDnaiList].negative {
			dnaiFilter = bson.M{
				mongoOpNot: dnaiFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), dnaiFilter)
	}
}

func addUpfIwkEpsFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-30] upf-iwk-eps-ind
	if queryParameters[queryParamUpfIwkEpsInd] != nil {
		var upfIwkEpsIndFilter bson.M
		// upfIwkEpsInd := queryParameters["upf-iwk-eps-ind"].value
		if targetNfType == nfTypeUPF {
			upfIwkEpsIndFilter = bson.M{
				fieldUpfInfo: bson.M{
					mongoOpElemMatch: bson.M{
						"iwkepsind": true,
					},
				},
			}
		}
		if queryParameters[queryParamUpfIwkEpsInd].negative {
			upfIwkEpsIndFilter = bson.M{
				mongoOpNot: upfIwkEpsIndFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), upfIwkEpsIndFilter)
	}
}

func addChfSupportedPlmnFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string, targetNfType string) {
	// [Query-31] chf-supported-plmn
	if queryParameters[queryParamChfSupportedPlmn] != nil {
		var chfSupportedPlmnFilter bson.M
		chfSupportedPlmn := queryParameters[queryParamChfSupportedPlmn].value
		if targetNfType == nfTypeCHF {
			chfSupportedPlmnFilter = bson.M{
				mongoOpOr: []bson.M{
					{
						fieldChfInfo: bson.M{
							mongoOpElemMatch: bson.M{
								"plmnrangelist": bson.M{
									mongoOpElemMatch: bson.M{
										fieldStart: bson.M{
											mongoOpLte: chfSupportedPlmn,
										},
										fieldEnd: bson.M{
											mongoOpGte: chfSupportedPlmn,
										},
									},
								},
							},
						},
					},
					{
						fieldChfInfoPlmnRangeList: bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		}
		if queryParameters[queryParamChfSupportedPlmn].negative {
			chfSupportedPlmnFilter = bson.M{
				mongoOpNot: chfSupportedPlmnFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), chfSupportedPlmnFilter)
	}
}

func addPreferredLocalityFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) {
	// [Query-32]  preferred-locality
	// TODO: if no match
	if queryParameters[queryParamPreferredLocality] != nil {
		preferredLocality := queryParameters[queryParamPreferredLocality].value
		preferredLocalityFilter := bson.M{
			"locality": preferredLocality,
		}
		if queryParameters[queryParamPreferredLocality].negative {
			preferredLocalityFilter = bson.M{
				mongoOpNot: preferredLocalityFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), preferredLocalityFilter)
	}
}

func addAccessTypeFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) {
	// [Query-33] access-type
	if queryParameters[queryParamAccessType] != nil {
		accessType := queryParameters[queryParamAccessType].value
		accessTypeFilter := bson.M{
			fieldSmfInfo: bson.M{
				mongoOpElemMatch: bson.M{
					"accesstype": accessType,
				},
			},
		}
		if queryParameters[queryParamAccessType].negative {
			accessTypeFilter = bson.M{
				mongoOpNot: accessTypeFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), accessTypeFilter)
	}
}

func addSupportedFeaturesFilter(queryParameters map[string]*AtomElem, filter bson.M, logicalOperator string) {
	// [Query-34] supported-features
	if queryParameters[queryParamSupportedFeatures] != nil {
		supportedFeatures := queryParameters[queryParamSupportedFeatures].value
		supportedFeaturesFilter := bson.M{
			fieldNfServices: bson.M{
				mongoOpElemMatch: bson.M{
					"supportedfeatures": supportedFeatures,
				},
			},
		}
		if queryParameters[queryParamSupportedFeatures].negative {
			supportedFeaturesFilter = bson.M{
				mongoOpNot: supportedFeaturesFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), supportedFeaturesFilter)
	}
}

func GetRequesterAndTargetNfTypeGivenQueryParameters(queryParameters url.Values) (requesterNfType, targetNfType string) {
	requesterNfType, targetNfType = nfTypeUnknown, nfTypeUnknown
	if queryParameters[queryParamRequesterNFType] != nil {
		requesterNfType = fmt.Sprint(queryParameters[queryParamRequesterNFType][0])
	}
	if queryParameters[queryParamTargetNFType] != nil {
		targetNfType = fmt.Sprint(queryParameters[queryParamTargetNFType][0])
	}
	return requesterNfType, targetNfType
}
