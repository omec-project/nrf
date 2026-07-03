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
	"github.com/omec-project/nrf/logger"
	stats "github.com/omec-project/nrf/metrics"
	"github.com/omec-project/nrf/util"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
)

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
	if normalized.Get("snssais") == "" && hasExplodedDiscoveryQueryParam(normalized, "snssais") {
		if value, ok := marshalExplodedSnssaiList(normalized, "snssais"); ok {
			normalized.Set("snssais", value)
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
			logger.DiscoveryLog.Warnln("unmarshal error in snssaiByteArray:", err)
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
			"snssais": bson.M{
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

	if queryParameters["complexQuery"] != nil {
		// IF SUPPORT COMPLEX QUERY
		// translate raw data to complexQuery structure
		complexQuery := queryParameters["complexQuery"][0]
		complexQueryStruct := &models.ComplexQuery{}
		err := json.Unmarshal([]byte(complexQuery), complexQueryStruct)
		if err != nil {
			logger.DiscoveryLog.Warnln("unmarshal complexQuery Error:", err)
		}
		// Check either CNF or DNF
		if complexQueryStruct.Cnf != nil && complexQueryStruct.Dnf != nil {
			problemDetails = utils.ProblemDetailsWithCause("Invalid Parameter", http.StatusBadRequest, "CNF and DNF are mutually exclusive", utils.CauseInvalidRequest)
			problemDetails.SetInvalidParams([]models.InvalidParam{
				{Param: "complexQuery"},
			})
			return nil, problemDetails
		}
	}

	// Check ComplexQuery (FOR REPORT PROBLEM!)

	// Build Query Filter
	filter := buildFilter(queryParameters)
	logger.DiscoveryLog.Debugln("query filter:", filter)

	// Use the filter to find documents
	nfProfilesRaw, err := dbadapter.DBClient.RestfulAPIGetMany("NfProfile", filter)
	if err != nil {
		logger.DiscoveryLog.Warnln("NF profile query error:", err)
	}
	logger.DiscoveryLog.Debugf("primary discovery raw count: %d", len(nfProfilesRaw))

	// nfProfile data for response
	var nfProfilesStruct []models.NFProfileDiscovery

	nfProfilesStruct, err = util.Decode(nfProfilesRaw, time.RFC3339)
	if err != nil {
		logger.DiscoveryLog.Warnln("NF Profile Raw decode error:", err)
	}
	logger.DiscoveryLog.Debugf("primary discovery decoded count: %d", len(nfProfilesStruct))

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

	// sort nfprofiles based on timestamp
	sort.Slice(nfProfilesRaw, func(i, j int) bool {
		var updatedTimeVal time.Time
		if nfProfilesRaw[i]["expireAt"] == nil {
			return false
		}
		updatedTimeVal = nfProfilesRaw[j]["expireAt"].(primitive.DateTime).Time()

		return nfProfilesRaw[i]["expireAt"].(primitive.DateTime).Time().Before(updatedTimeVal)
	})

	// handle ipv4 & ipv6
	if queryParameters[queryParamTargetNFType][0] == "BSF" {
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
	// Build SearchResult model
	searchResult := models.NewSearchResult(100, nfProfilesStruct)

	return searchResult, nil
}

func loadDiscoveryProfilesFromURIList(queryParameters url.Values) ([]models.NFProfileDiscovery, error) {
	targetNfType := queryParameters[queryParamTargetNFType][0]
	uriListRaw, err := dbadapter.DBClient.RestfulAPIGetOne("urilist", bson.M{"nfType": targetNfType})
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

	profileListRaw, err := dbadapter.DBClient.RestfulAPIGetMany("NfProfile", bson.M{
		"nfinstanceid": bson.M{"$in": uniqueInstanceIDs},
	})
	if err != nil {
		return nil, err
	}

	profilesByInstanceID := make(map[string]map[string]interface{}, len(profileListRaw))
	for _, profileRaw := range profileListRaw {
		if profileRaw == nil {
			continue
		}
		if nfInstanceID, ok := profileRaw["nfinstanceid"].(string); ok && nfInstanceID != "" {
			profilesByInstanceID[nfInstanceID] = profileRaw
		}
	}

	profiles := make([]models.NFProfileDiscovery, 0, len(orderedInstanceIDs))
	for _, nfInstanceID := range orderedInstanceIDs {
		profileRaw := profilesByInstanceID[nfInstanceID]

		if profileRaw == nil {
			continue
		}

		decodedProfiles, decodeErr := util.Decode([]any{profileRaw}, time.RFC3339)
		if decodeErr != nil {
			logger.DiscoveryLog.Warnf("fallback profile decode error for %s: %v", nfInstanceID, decodeErr)
			continue
		}
		if len(decodedProfiles) == 0 {
			continue
		}
		decodedProfile := decodedProfiles[0]
		if decodedProfile.GetNfInstanceId() == "" {
			continue
		}

		profiles = append(profiles, decodedProfile)
	}

	return profiles, nil
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
		"$and": []bson.M{},
	}

	targetNfType := ""

	// [Query-1] target-nf-type
	if len(queryParameters[queryParamTargetNFType]) > 0 {
		targetNfType = queryParameters[queryParamTargetNFType][0]
		if targetNfType != "" {
			targetNfTypeFilter := bson.M{
				"nftype": targetNfType,
			}
			filter["$and"] = append(filter["$and"].([]bson.M), targetNfTypeFilter)
		}
	}

	// [Query-2] request-nf-type
	requesterNfType := queryParameters[queryParamRequesterNFType][0]
	if requesterNfType != "" {
		requesterNfTypeFilter := bson.M{
			"$or": []bson.M{
				{"allowednftypes": requesterNfType},
				{"allowednftypes": nil},
			},
		}
		filter["$and"] = append(filter["$and"].([]bson.M), requesterNfTypeFilter)
	}

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
			"nfservices": bson.M{
				mongoOpElemMatch: bson.M{
					"servicename": bson.M{
						// get all service in array
						"$in": serviceNamesBsonArray,
					},
					// the service need to be registered
					"nfservicestatus": "REGISTERED",
				},
			},
		}
		filter["$and"] = append(filter["$and"].([]bson.M), serviceNamesFilter)
	}

	// [Query-4] requester-nfinstance-fqdn
	if queryParameters["requester-nf-instance-fqdn"] != nil {
		requesterNfinstanceFqdn := queryParameters["requester-nf-instance-fqdn"][0]

		requesterNfinstanceFqdnFilter := bson.M{
			"$or": []bson.M{
				{
					"nfservices": bson.M{
						mongoOpElemMatch: bson.M{
							"allowednfdomains": requesterNfinstanceFqdn,
						},
					},
				},
				{ // if not provided, allow any.
					"nfservices": bson.M{
						mongoOpElemMatch: bson.M{
							"allowednfdomains": bson.M{
								mongoOpExists: false,
							},
						},
					},
				},
			},
		}
		filter["$and"] = append(filter["$and"].([]bson.M), requesterNfinstanceFqdnFilter)
	}

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

				targetPlmnListBsonArray = append(targetPlmnListBsonArray, bson.M{"plmnlist": bson.M{mongoOpElemMatch: targetPlmnBsonM}})
			}
		}

		targetPlmnListFilter := bson.M{
			"$or": targetPlmnListBsonArray,
		}

		filter["$and"] = append(filter["$and"].([]bson.M), targetPlmnListFilter)
	}

	// [Query-6] requester-plmn-list
	// if queryParameters["requester-plmn-list"] != nil {
	// requesterPlmnPist := queryParameters["requester-plmn-list"][0]
	// TODO
	// }

	// [Query-7] target-nf-instance-id
	if queryParameters["target-nf-instance-id"] != nil {
		targetNfInstanceid := queryParameters["target-nf-instance-id"][0]
		nfInstanceIdFilter := bson.M{
			"nfinstanceid": targetNfInstanceid,
		}
		filter["$and"] = append(filter["$and"].([]bson.M), nfInstanceIdFilter)
	}

	// [Query-8] target-nf-fqdn
	if queryParameters[queryParamTargetNfFqdn] != nil {
		targetNfFqdn := queryParameters[queryParamTargetNfFqdn][0]
		fqdnFilter := bson.M{
			"fqdn": targetNfFqdn,
		}
		filter["$and"] = append(filter["$and"].([]bson.M), fqdnFilter)
	}

	// [Query-9] hnrf-uri
	// for Roaming

	// [Query-10] snssais
	// Pattern: '^[A-Fa-f0-9]{6}$'
	if queryParameters["snssais"] != nil {
		snssais := queryParameters["snssais"][0]
		snssaisFilters := buildSnssaisElemMatchFilters(snssais)
		var snssaisBsonArray bson.A
		for _, snssaisFilter := range snssaisFilters {
			snssaisBsonArray = append(snssaisBsonArray, snssaisFilter)
		}

		// if not assign, serve all NF
		snssaisBsonArray = append(snssaisBsonArray, bson.M{"snssais": bson.M{mongoOpExists: false}})

		snssaisFilter := bson.M{
			"$or": snssaisBsonArray,
		}

		filter["$and"] = append(filter["$and"].([]bson.M), snssaisFilter)
	}

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
		filter["$and"] = append(filter["$and"].([]bson.M), nsiListFilter)
	}

	// [Query-12] dnn
	if queryParameters["dnn"] != nil {
		dnn := queryParameters["dnn"][0]
		var dnnFilter bson.M
		switch targetNfType {
		case "SMF":
			dnnFilter = bson.M{
				"smfinfo.snssaismfinfolist": bson.M{
					mongoOpElemMatch: bson.M{
						"dnnsmfinfolist": bson.M{
							mongoOpElemMatch: bson.M{
								"$or": []bson.M{
									{"dnn": dnn},
									{"dnn.string": dnn},
								},
							},
						},
					},
				},
			}
		case "UPF":
			dnnFilter = bson.M{
				"upfinfo.snssaiupfinfolist": bson.M{
					mongoOpElemMatch: bson.M{
						"dnnupfinfolist": bson.M{
							mongoOpElemMatch: bson.M{
								"dnn": dnn,
							},
						},
					},
				},
			}
		case "BSF":
			dnnFilter = bson.M{
				"$or": []bson.M{
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
		case "PCF":
			dnnFilter = bson.M{
				"$or": []bson.M{
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
		filter["$and"] = append(filter["$and"].([]bson.M), dnnFilter)
	}

	// [Query-13] smf-serving-area
	if queryParameters[queryParamSmfServingArea] != nil {
		var smfServingAreaFilter bson.M
		smfServingArea := queryParameters[queryParamSmfServingArea][0]
		if targetNfType == "UPF" {
			smfServingAreaFilter = bson.M{
				"$or": []bson.M{
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
		filter["$and"] = append(filter["$and"].([]bson.M), smfServingAreaFilter)
	}

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
		case "SMF":
			taiFilter = bson.M{
				"smfinfo.tailist": bson.M{
					mongoOpElemMatch: taiBsonM,
				},
			}
		case "AMF":
			taiFilter = bson.M{
				"amfinfo.tailist": bson.M{
					mongoOpElemMatch: taiBsonM,
				},
			}
		}
		filter["$and"] = append(filter["$and"].([]bson.M), taiFilter)
	}

	// [Query-15] amf-region-id
	if queryParameters[queryParamAmfRegionID] != nil {
		if targetNfType == "AMF" {
			amfRegionId := queryParameters[queryParamAmfRegionID][0]
			amfRegionIdFilter := bson.M{
				"amfinfo.amfregionid": amfRegionId,
			}
			filter["$and"] = append(filter["$and"].([]bson.M), amfRegionIdFilter)
		}
	}

	// [Query-16] amf-set-id
	if queryParameters[queryParamAmfSetID] != nil {
		if targetNfType == "AMF" {
			amfSetId := queryParameters[queryParamAmfSetID][0]
			amfSetIdFilter := bson.M{
				"amfinfo.amfsetid": amfSetId,
			}
			filter["$and"] = append(filter["$and"].([]bson.M), amfSetIdFilter)
		}
	}

	// Query-17: guami
	// TODO: NOTE[1]
	if queryParameters["guami"] != nil {
		if targetNfType == "AMF" {
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

			filter["$and"] = append(filter["$and"].([]bson.M), guamiFilter)
		}
	}

	// [Query-18] supi
	var supi string
	if queryParameters["supi"] != nil {
		var supiFilter bson.M
		supi = queryParameters["supi"][0]
		supi = supi[5:]
		switch targetNfType {
		case "PCF":
			supiFilter = bson.M{
				"$or": []bson.M{
					{
						"pcfinfo.supiranges": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": supi,
								},
								"end": bson.M{
									"$gte": supi,
								},
							},
						},
					},
					{
						"pcfinfo.supiranges": nil,
					},
					{
						"pcfinfo.supiranges": bson.M{
							mongoOpExists: false,
						},
					},
				},
			}
		case "CHF":
			supiFilter = bson.M{
				"$or": []bson.M{
					{
						"chfinfo.supirangelist": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": supi,
								},
								"end": bson.M{
									"$gte": supi,
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
		case "AUSF":
			supiFilter = bson.M{
				"$or": []bson.M{
					{
						"ausfinfo.supiranges": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": supi,
								},
								"end": bson.M{
									"$gte": supi,
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
		case "UDM":
			supiFilter = bson.M{
				"$or": []bson.M{
					{
						fieldUdmInfoSupiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": supi,
								},
								"end": bson.M{
									"$gte": supi,
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
		case "UDR":
			supiFilter = bson.M{
				"$or": []bson.M{
					{
						fieldUdrInfoSupiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": supi,
								},
								"end": bson.M{
									"$gte": supi,
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
		filter["$and"] = append(filter["$and"].([]bson.M), supiFilter)
	}

	// [Query-19] ue-ipv4-address
	if queryParameters[queryParamUeIpv4Address] != nil {
		var ueIpv4AddressFilter bson.M
		if targetNfType == "BSF" {
			ueIpv4Address := queryParameters[queryParamUeIpv4Address][0]
			ueIpv4AddressNumber := context.Ipv4ToInt(ueIpv4Address)
			ueIpv4AddressFilter = bson.M{
				"$or": []bson.M{
					{
						"bsfinfo.ipv4addressranges": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": strconv.Itoa(int(ueIpv4AddressNumber)),
								},
								"end": bson.M{
									"$gte": strconv.Itoa(int(ueIpv4AddressNumber)),
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
		filter["$and"] = append(filter["$and"].([]bson.M), ueIpv4AddressFilter)
	}

	// [Query-20] ip-domain
	if queryParameters[queryParamIpDomain] != nil {
		var ipDomainFilter bson.M
		if targetNfType == "BSF" {
			ipDomain := queryParameters[queryParamIpDomain][0]
			ipDomainFilter = bson.M{
				"$or": []bson.M{
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
		filter["$and"] = append(filter["$and"].([]bson.M), ipDomainFilter)
	}

	// [Query-21] ue-ipv6-prefix
	if queryParameters[queryParamUeIpv6Prefix] != nil {
		var ueIpv6PrefixFilter bson.M
		if targetNfType == "BSF" {
			ueIpv6Prefix := queryParameters[queryParamUeIpv6Prefix][0]
			ueIpv6PrefixNumber := context.Ipv6ToInt(ueIpv6Prefix)
			ueIpv6PrefixFilter = bson.M{
				"$or": []bson.M{
					{
						"bsfinfo.ipv6prefixranges": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": ueIpv6PrefixNumber.String(),
								},
								"end": bson.M{
									"$gte": ueIpv6PrefixNumber.String(),
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
		filter["$and"] = append(filter["$and"].([]bson.M), ueIpv6PrefixFilter)
	}

	// [Query-22] pgw-ind
	if queryParameters[queryParamPgwInd] != nil {
		pgwInd := queryParameters[queryParamPgwInd][0]
		if pgwInd == "true" {
			pgwIndFilter := bson.M{
				"smfinfo.pgwfqdn": bson.M{
					mongoOpExists: true,
				},
			}
			filter["$and"] = append(filter["$and"].([]bson.M), pgwIndFilter)
		}
	}

	// [Query-23] pgw
	if queryParameters["pgw"] != nil {
		pgw := queryParameters["pgw"][0]
		pgwFilter := bson.M{
			"smfinfo.pgwfqdn": pgw,
		}
		filter["$and"] = append(filter["$and"].([]bson.M), pgwFilter)
	}

	// [Query-24] gpsi
	if queryParameters["gpsi"] != nil {
		var gpsiFilter bson.M
		gpsi := queryParameters["gpsi"][0]
		gpsi = gpsi[7:]
		switch targetNfType {
		case "CHF":
			gpsiFilter = bson.M{
				"$or": []bson.M{
					{
						"chfinfo.gpsirangelist": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": gpsi,
								},
								"end": bson.M{
									"$gte": gpsi,
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
		case "UDM":
			gpsiFilter = bson.M{
				"$or": []bson.M{
					{
						fieldUdmInfoGpsiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": gpsi,
								},
								"end": bson.M{
									"$gte": gpsi,
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
		case "UDR":
			gpsiFilter = bson.M{
				"$or": []bson.M{
					{
						fieldUdrInfoGpsiRanges: bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": gpsi,
								},
								"end": bson.M{
									"$gte": gpsi,
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
		filter["$and"] = append(filter["$and"].([]bson.M), gpsiFilter)
	}

	// [Query-25] external-group-identity
	if queryParameters[queryParamExternalGroupIdentity] != nil {
		var externalGroupIdentityFilter bson.M
		externalGroupIdentity := queryParameters[queryParamExternalGroupIdentity][0]

		encodedGroupId := context.EncodeGroupId(externalGroupIdentity)
		switch targetNfType {
		case "UDM":
			externalGroupIdentityFilter = bson.M{
				"$or": []bson.M{
					{
						fieldUdmExtGrpIDRanges: bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": encodedGroupId,
								},
								"end": bson.M{
									"$gte": encodedGroupId,
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
		case "UDR":
			externalGroupIdentityFilter = bson.M{
				"$or": []bson.M{
					{
						fieldUdrExtGroupIDRanges: bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": encodedGroupId,
								},
								"end": bson.M{
									"$gte": encodedGroupId,
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
		filter["$and"] = append(filter["$and"].([]bson.M), externalGroupIdentityFilter)
	}

	// [Query-26] data-set
	if queryParameters[queryParamDataSet] != nil {
		var dataSetFilter bson.M
		dataSet := queryParameters[queryParamDataSet]
		if targetNfType == "UDR" {
			dataSetFilter = bson.M{
				"$or": []bson.M{
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
		filter["$and"] = append(filter["$and"].([]bson.M), dataSetFilter)
	}

	// [Query-27] routing-indicator
	if queryParameters[queryParamRoutingIndicator] != nil {
		var routingIndicatorFilter bson.M
		routingIndicator := queryParameters[queryParamRoutingIndicator][0]
		switch targetNfType {
		case "AUSF":
			routingIndicatorFilter = bson.M{
				"$or": []bson.M{
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
		case "UDM":
			routingIndicatorFilter = bson.M{
				"$or": []bson.M{
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
		filter["$and"] = append(filter["$and"].([]bson.M), routingIndicatorFilter)
	}

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
		case "UDR":
			groupIdListFilter = bson.M{
				"udrinfo.groupid": bson.M{
					"$in": groupIdListBsonArray,
				},
			}
		case "UDM":
			groupIdListFilter = bson.M{
				"udminfo.groupid": bson.M{
					"$in": groupIdListBsonArray,
				},
			}
		case "AUSF":
			groupIdListFilter = bson.M{
				"ausfinfo.groupid": bson.M{
					"$in": groupIdListBsonArray,
				},
			}
		}
		filter["$and"] = append(filter["$and"].([]bson.M), groupIdListFilter)
	}

	// [Query-29] dnai-list
	if queryParameters[queryParamDnaiList] != nil {
		var dnaiFilter bson.M
		dnaiList := queryParameters[queryParamDnaiList][0]
		dnaiListSplit := strings.Split(dnaiList, ",")
		var dnaiListBsonArray bson.A

		for _, v := range dnaiListSplit {
			dnaiListBsonArray = append(dnaiListBsonArray, v)
		}
		if targetNfType == "UPF" {
			dnaiFilter = bson.M{
				"upfinfo.snssaiupfinfolist": bson.M{
					mongoOpElemMatch: bson.M{
						"dnnupfinfolist": bson.M{
							mongoOpElemMatch: bson.M{
								"dnailist": bson.M{
									"$in": dnaiListBsonArray,
								},
							},
						},
					},
				},
			}
		}
		filter["$and"] = append(filter["$and"].([]bson.M), dnaiFilter)
	}

	// [Query-30] upf-iwk-eps-ind
	if queryParameters[queryParamUpfIwkEpsInd] != nil {
		var upfIwkEpsIndFilter bson.M
		// upfIwkEpsInd := queryParameters["upf-iwk-eps-ind"][0]
		if targetNfType == "UPF" {
			upfIwkEpsIndFilter = bson.M{
				"upfinfo.iwkepsind": true,
			}
		}
		filter["$and"] = append(filter["$and"].([]bson.M), upfIwkEpsIndFilter)
	}

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

		if targetNfType == "CHF" {
			chfSupportedPlmnFilter = bson.M{
				"$or": []bson.M{
					{
						fieldChfInfoPlmnRangeList: bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": encodedchfSupportedPlmn,
								},
								"end": bson.M{
									"$gte": encodedchfSupportedPlmn,
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
		filter["$and"] = append(filter["$and"].([]bson.M), chfSupportedPlmnFilter)
	}

	// [Query-32]  preferred-locality
	// TODO: if no match
	if queryParameters[queryParamPreferredLocality] != nil {
		preferredLocality := queryParameters[queryParamPreferredLocality][0]
		preferredLocalityFilter := bson.M{
			"locality": preferredLocality,
		}
		filter["$and"] = append(filter["$and"].([]bson.M), preferredLocalityFilter)
	}

	// [Query-33] access-type
	if queryParameters[queryParamAccessType] != nil {
		accessType := queryParameters[queryParamAccessType][0]
		accessTypeFilter := bson.M{
			"$or": []bson.M{
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
		filter["$and"] = append(filter["$and"].([]bson.M), accessTypeFilter)
	}

	// [Query-34] supported-features
	if queryParameters[queryParamSupportedFeatures] != nil {
		supportedFeatures := queryParameters[queryParamSupportedFeatures][0]
		supportedFeaturesFilter := bson.M{
			"nfservices": bson.M{
				mongoOpElemMatch: bson.M{
					"supportedfeatures": supportedFeatures,
				},
			},
		}
		filter["$and"] = append(filter["$and"].([]bson.M), supportedFeaturesFilter)
	}

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
		filter["$and"] = append(filter["$and"].([]bson.M), complexQueryFilter)
	}
	return filter
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
			"$and": []bson.M{},
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

			filter["$and"] = append(filter["$and"].([]bson.M), cnfUnitFilter)
		}
	} else {
		filter = bson.M{
			"$or": []bson.M{},
		}
	}
	return filter
}

func complexQueryFilterSubprocess(queryParameters map[string]*AtomElem, complexQueryType string) bson.M {
	var filter bson.M
	var logicalOperator string

	switch complexQueryType {
	case COMPLEX_QUERY_TYPE_CNF:
		logicalOperator = "$or"
	case COMPLEX_QUERY_TYPE_DNF:
		logicalOperator = "$and"
	}

	filter = bson.M{
		logicalOperator: []bson.M{},
	}

	// [Query-1] target-nf-type
	var targetNfType string
	if targetNfTypeParam, ok := queryParameters[queryParamTargetNFType]; ok && targetNfTypeParam != nil {
		var targetNfTypeFilter bson.M
		targetNfType = targetNfTypeParam.value
		negative := targetNfTypeParam.negative
		if negative {
			targetNfTypeFilter = bson.M{
				"nftype": bson.M{
					"$ne": targetNfType,
				},
			}
		} else if !negative {
			targetNfTypeFilter = bson.M{
				"nftype": targetNfType,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), targetNfTypeFilter)
	}

	// [Query-2] requester-nf-type
	// requesterNfType := queryParameters["requester-nf-type"].value
	// TODO

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
				"nfservices": bson.M{
					mongoOpElemMatch: bson.M{
						"servicename": bson.M{
							// get all service in array
							"$nin": serviceNamesBsonArray,
						},
						// the service need to be registered
						"nfservicestatus": "REGISTERED",
					},
				},
			}
		} else if !negative {
			serviceNamesFilter = bson.M{
				"nfservices": bson.M{
					mongoOpElemMatch: bson.M{
						"servicename": bson.M{
							// get all service in array
							"$in": serviceNamesBsonArray,
						},
						// the service need to be registered
						"nfservicestatus": "REGISTERED",
					},
				},
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), serviceNamesFilter)
	}

	// [Query-4] requester-nfinstance-fqdn
	if queryParameters[queryParamRequesterNfInstanceFqdn] != nil {
		var requesterNfinstanceFqdnFilter bson.M
		requesterNfinstanceFqdn := queryParameters[queryParamRequesterNfInstanceFqdn].value

		negative := queryParameters[queryParamRequesterNfInstanceFqdn].negative
		if negative {
			requesterNfinstanceFqdnFilter = bson.M{
				"nfservices": bson.M{
					mongoOpElemMatch: bson.M{
						"allowednfdomains": requesterNfinstanceFqdn,
					},
				},
			}
		} else if !negative {
			requesterNfinstanceFqdnFilter = bson.M{
				"nfservices": bson.M{
					mongoOpElemMatch: bson.M{
						"allowednfdomains": bson.M{
							"$ne": requesterNfinstanceFqdn,
						},
					},
				},
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), requesterNfinstanceFqdnFilter)
	}

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
					logger.DiscoveryLog.Warnln("unmarshal error in targetPlmnByteArray:", err)
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
				"plmnlist": bson.M{
					"$nin": targetPlmnListBsonArray,
				},
			}
		} else if !negative {
			targetPlmnListFilter = bson.M{
				"plmnlist": bson.M{
					"$in": targetPlmnListBsonArray,
				},
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), targetPlmnListFilter)
	}

	// [Query-6] requester-plmn-list
	// if queryParameters["requester-plmn-list"] != nil {
	// requesterPlmnPist := queryParameters["requester-plmn-list"].value
	// TODO
	// }

	// [Query-7] target-nf-instanceid
	if queryParameters[queryParamTargetNfInstanceID] != nil {
		targetNfInstanceid := queryParameters[queryParamTargetNfInstanceID].value
		var nfInstanceIdFilter bson.M

		negative := queryParameters[queryParamTargetNfInstanceID].negative
		if negative {
			nfInstanceIdFilter = bson.M{
				"nfinstanceid": bson.M{
					"$ne": targetNfInstanceid,
				},
			}
		} else if !negative {
			nfInstanceIdFilter = bson.M{
				"nfinstanceid": targetNfInstanceid,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), nfInstanceIdFilter)
	}

	// [Query-8] target-nf-fqdn
	if queryParameters[queryParamTargetNfFqdn] != nil {
		targetNfFqdn := queryParameters[queryParamTargetNfFqdn].value
		fqdnFilter := bson.M{
			"fqdn": targetNfFqdn,
		}
		if queryParameters[queryParamTargetNfFqdn].negative {
			fqdnFilter = bson.M{
				"fqdn": bson.M{
					"$ne": targetNfFqdn,
				},
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), fqdnFilter)
	}

	// [Query-9] hnrf-uri
	// for Roaming

	// [Query-10] snssais
	// Pattern: '^[A-Fa-f0-9]{6}$'
	if queryParameters["snssais"] != nil {
		snssaisFilters := buildSnssaisElemMatchFilters(queryParameters["snssais"].value)
		if len(snssaisFilters) > 0 {
			var snssaisFilter bson.M
			switch len(snssaisFilters) {
			case 1:
				snssaisFilter = snssaisFilters[0]
			default:
				snssaisFilter = bson.M{
					"$or": snssaisFilters,
				}
			}
			if queryParameters["snssais"].negative {
				snssaisFilter = bson.M{
					"$nor": snssaisFilters,
				}
			}
			filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), snssaisFilter)
		}
	}

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
				"$not": nsiListFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), nsiListFilter)
	}

	// [Query-12] dnn
	if queryParameters["dnn"] != nil {
		dnn := queryParameters["dnn"].value
		var dnnFilter bson.M
		switch targetNfType {
		case "SMF":
			dnnFilter = bson.M{
				"smfinfo.snssaismfinfolist": bson.M{
					mongoOpElemMatch: bson.M{
						"dnnsmfinfolist": bson.M{
							mongoOpElemMatch: bson.M{
								"$or": []bson.M{
									{"dnn": dnn},
									{"dnn.string": dnn},
								},
							},
						},
					},
				},
			}
		case "UPF":
			dnnFilter = bson.M{
				"upfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"snssaiupfinfolist": bson.M{
							mongoOpElemMatch: bson.M{
								"dnnupfinfolist": bson.M{
									mongoOpElemMatch: bson.M{
										"dnn": dnn,
									},
								},
							},
						},
					},
				},
			}
		case "BSF":
			dnnFilter = bson.M{
				"bsfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"dnnlist": dnn,
					},
				},
			}
		}
		if queryParameters["dnn"].negative {
			dnnFilter = bson.M{
				"$not": dnnFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), dnnFilter)
	}

	// [Query-13] smf-serving-area
	if queryParameters[queryParamSmfServingArea] != nil {
		var smfServingAreaFilter bson.M
		smfServingArea := queryParameters[queryParamSmfServingArea].value
		if targetNfType == "UPF" {
			smfServingAreaFilter = bson.M{
				"upfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"smfservingarea": smfServingArea,
					},
				},
			}
		}
		if queryParameters[queryParamSmfServingArea].negative {
			smfServingAreaFilter = bson.M{
				"$not": smfServingAreaFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), smfServingAreaFilter)
	}

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
		case "SMF":
			taiFilter = bson.M{
				"smfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"tailist": taiBsonM,
					},
				},
			}
		case "AMF":
			taiFilter = bson.M{
				"amfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"tailist": taiBsonM,
					},
				},
			}
		}
		if queryParameters["tai"].negative {
			taiFilter = bson.M{
				"$not": taiFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), taiFilter)
	}

	// [Query-15] amf-region-id
	if queryParameters[queryParamAmfRegionID] != nil {
		var amfRegionIdFilter bson.M
		if targetNfType == "AMF" {
			amfRegionId := queryParameters[queryParamAmfRegionID].value
			amfRegionIdFilter = bson.M{
				"amfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"amfregionid": amfRegionId,
					},
				},
			}
		}
		if queryParameters[queryParamAmfRegionID].negative {
			amfRegionIdFilter = bson.M{
				"$not": amfRegionIdFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), amfRegionIdFilter)
	}

	// [Query-16] amf-set-id
	if queryParameters[queryParamAmfSetID] != nil {
		var amfSetIdFilter bson.M
		if targetNfType == "AMF" {
			amfSetId := queryParameters[queryParamAmfSetID].value
			amfSetIdFilter = bson.M{
				"amfinfo": bson.M{
					mongoOpElemMatch: bson.M{ // TOCHECK : elemMatch
						"amfsetid": amfSetId,
					},
				},
			}
		}
		if queryParameters[queryParamAmfSetID].negative {
			amfSetIdFilter = bson.M{
				"$not": amfSetIdFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), amfSetIdFilter)
	}

	// Query-17: guami
	// TODO: NOTE[1]
	if queryParameters["guami"] != nil {
		var guamiFilter bson.M
		if targetNfType == "AMF" {
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
				"amfinfo": bson.M{
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
				"$not": guamiFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), guamiFilter)
	}

	// [Query-18] supi
	var supi string
	if queryParameters["supi"] != nil {
		var supiFilter bson.M
		supi = queryParameters["supi"].value
		switch targetNfType {
		case "PCF":
			supiFilter = bson.M{
				"pcfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"supiranges": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": supi[0],
								},
								"end": bson.M{
									"$gte": supi[0],
								},
							},
						},
					},
				},
			}
		case "CHF":
			supiFilter = bson.M{
				"chfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"supiranges": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": supi[0],
								},
								"end": bson.M{
									"$gte": supi[0],
								},
							},
						},
					},
				},
			}
		case "AUSF":
			supiFilter = bson.M{
				"ausfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"supiranges": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": supi[0],
								},
								"end": bson.M{
									"$gte": supi[0],
								},
							},
						},
					},
				},
			}
		case "UDM":
			supiFilter = bson.M{
				"udminfo": bson.M{
					mongoOpElemMatch: bson.M{
						"supiranges": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": supi[0],
								},
								"end": bson.M{
									"$gte": supi[0],
								},
							},
						},
					},
				},
			}
		case "UDR":
			supiFilter = bson.M{
				"udrinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"supiranges": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": supi[0],
								},
								"end": bson.M{
									"$gte": supi[0],
								},
							},
						},
					},
				},
			}
		}
		if queryParameters["supi"].negative {
			supiFilter = bson.M{
				"$not": supiFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), supiFilter)
	}

	// [Query-19] ue-ipv4-address
	if queryParameters[queryParamUeIpv4Address] != nil {
		var ueIpv4AddressFilter bson.M
		if targetNfType == "BSF" {
			ueIpv4Address := queryParameters[queryParamUeIpv4Address].value
			ueIpv4AddressNumber := context.Ipv4ToInt(ueIpv4Address)
			ueIpv4AddressFilter = bson.M{
				"bsfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"ipv4addressranges": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": ueIpv4AddressNumber,
								},
								"end": bson.M{
									"$gte": ueIpv4AddressNumber,
								},
							},
						},
					},
				},
			}
		}
		if queryParameters[queryParamUeIpv4Address].negative {
			ueIpv4AddressFilter = bson.M{
				"$not": ueIpv4AddressFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), ueIpv4AddressFilter)
	}

	// [Query-20] ip-domain
	if queryParameters[queryParamIpDomain] != nil {
		var ipDomainFilter bson.M
		if targetNfType == "BSF" {
			ipDomain := queryParameters[queryParamIpDomain].value
			ipDomainFilter = bson.M{
				"bsfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"ipdomainlist": ipDomain,
					},
				},
			}
		}
		if queryParameters[queryParamIpDomain].negative {
			ipDomainFilter = bson.M{
				"$not": ipDomainFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), ipDomainFilter)
	}

	// [Query-21] ue-ipv6-prefix
	if queryParameters[queryParamUeIpv6Prefix] != nil {
		var ueIpv6PrefixFilter bson.M
		if targetNfType == "BSF" {
			ueIpv6Prefix := queryParameters[queryParamUeIpv6Prefix].value
			ueIpv6PrefixNumber := context.Ipv6ToInt(ueIpv6Prefix)
			ueIpv6PrefixFilter = bson.M{
				"bsfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"ipv6prefixranges": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": ueIpv6PrefixNumber,
								},
								"end": bson.M{
									"$gte": ueIpv6PrefixNumber,
								},
							},
						},
					},
				},
			}
		}
		if queryParameters[queryParamUeIpv6Prefix].negative {
			ueIpv6PrefixFilter = bson.M{
				"$not": ueIpv6PrefixFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), ueIpv6PrefixFilter)
	}

	// [Query-22] pgw-ind
	if queryParameters[queryParamPgwInd] != nil {
		var pgwIndFilter bson.M
		pgwInd := queryParameters[queryParamPgwInd].value
		if pgwInd == "true" {
			pgwIndFilter = bson.M{
				"smfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"pgwfqdn": bson.M{
							"$ne": "",
						},
					},
				},
			}
		}
		if queryParameters[queryParamPgwInd].negative {
			pgwIndFilter = bson.M{
				"$not": pgwIndFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), pgwIndFilter)
	}

	// [Query-23] pgw
	if queryParameters["pgw"] != nil {
		pgw := queryParameters["pgw"].value
		pgwFilter := bson.M{
			"smfinfo": bson.M{
				mongoOpElemMatch: bson.M{
					"pgwfqdn": pgw,
				},
			},
		}
		if queryParameters["pgw"].negative {
			pgwFilter = bson.M{
				"$not": pgwFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), pgwFilter)
	}

	// [Query-24] gpsi
	if queryParameters["gpsi"] != nil {
		var gpsiFilter bson.M
		gpsi := queryParameters["gpsi"].value
		switch targetNfType {
		case "CHF":
			gpsiFilter = bson.M{
				"chfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"gpsirangelist": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": gpsi,
								},
								"end": bson.M{
									"$gte": supi,
								},
							},
						},
					},
				},
			}
		case "UDM":
			gpsiFilter = bson.M{
				"udminfo": bson.M{
					mongoOpElemMatch: bson.M{
						"gpsirangelist": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": gpsi[0],
								},
								"end": bson.M{
									"$gte": supi[0],
								},
							},
						},
					},
				},
			}
		case "UDR":
			gpsiFilter = bson.M{
				"udrinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"gpsirangelist": bson.M{
							mongoOpElemMatch: bson.M{
								"start": bson.M{
									"$lte": gpsi[0],
								},
								"end": bson.M{
									"$gte": supi[0],
								},
							},
						},
					},
				},
			}
		}
		if queryParameters["gpsi"].negative {
			gpsiFilter = bson.M{
				"$not": gpsiFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), gpsiFilter)
	}

	// [Query-25] external-group-identity
	if queryParameters[queryParamExternalGroupIdentity] != nil {
		var externalGroupIdentityFilter bson.M
		externalGroupIdentity := queryParameters[queryParamExternalGroupIdentity].value
		switch targetNfType {
		case "UDM":
			externalGroupIdentityFilter = bson.M{
				"udminfo": bson.M{
					mongoOpElemMatch: bson.M{
						"groupid": externalGroupIdentity,
					},
				},
			}
		case "UDR":
			externalGroupIdentityFilter = bson.M{
				"udrinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"groupid": externalGroupIdentity,
					},
				},
			}
		}
		if queryParameters[queryParamExternalGroupIdentity].negative {
			externalGroupIdentityFilter = bson.M{
				"$not": externalGroupIdentityFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), externalGroupIdentityFilter)
	}

	// [Query-26] data-set
	if queryParameters[queryParamDataSet] != nil {
		var dataSetFilter bson.M
		dataSet := queryParameters[queryParamDataSet]
		if targetNfType == "UDR" {
			dataSetFilter = bson.M{
				"udrinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"supporteddatasets": dataSet,
					},
				},
			}
		}
		if queryParameters[queryParamDataSet].negative {
			dataSetFilter = bson.M{
				"$not": dataSetFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), dataSetFilter)
	}

	// [Query-27] routing-indicator
	if queryParameters[queryParamRoutingIndicator] != nil {
		var routingIndicatorFilter bson.M
		routingIndicator := queryParameters[queryParamRoutingIndicator].value
		switch targetNfType {
		case "AUSF":
			routingIndicatorFilter = bson.M{
				"ausfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"routingindicators": routingIndicator,
					},
				},
			}
		case "UDM":
			routingIndicatorFilter = bson.M{
				"udminfo": bson.M{
					mongoOpElemMatch: bson.M{
						"routingindicators": routingIndicator,
					},
				},
			}
		}
		if queryParameters[queryParamRoutingIndicator].negative {
			routingIndicatorFilter = bson.M{
				"$not": routingIndicatorFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), routingIndicatorFilter)
	}

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
		case "UDR":
			groupIdListFilter = bson.M{
				"udrinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"groupid": bson.M{
							"$in": groupIdListBsonArray,
						},
					},
				},
			}
		case "UDM":
			groupIdListFilter = bson.M{
				"udminfo": bson.M{
					mongoOpElemMatch: bson.M{
						"groupid": bson.M{
							"$in": groupIdListBsonArray,
						},
					},
				},
			}
		case "AUSF":
			groupIdListFilter = bson.M{
				"ausfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"groupid": bson.M{
							"$in": groupIdListBsonArray,
						},
					},
				},
			}
		}
		if queryParameters[queryParamGroupIDList].negative {
			groupIdListFilter = bson.M{
				"$not": groupIdListFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), groupIdListFilter)
	}

	// [Query-29] dnai-list
	if queryParameters[queryParamDnaiList] != nil {
		var dnaiFilter bson.M
		dnaiList := queryParameters[queryParamDnaiList].value
		dnaiListSplit := strings.Split(dnaiList, ",")
		var dnaiListBsonArray bson.A

		for _, v := range dnaiListSplit {
			dnaiListBsonArray = append(dnaiListBsonArray, v)
		}
		if targetNfType == "UPF" {
			dnaiFilter = bson.M{
				"upfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"snssaiupfinfolist": bson.M{
							mongoOpElemMatch: bson.M{
								"dnnupfinfolist": bson.M{
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
				"$not": dnaiFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), dnaiFilter)
	}

	// [Query-30] upf-iwk-eps-ind
	if queryParameters[queryParamUpfIwkEpsInd] != nil {
		var upfIwkEpsIndFilter bson.M
		// upfIwkEpsInd := queryParameters["upf-iwk-eps-ind"].value
		if targetNfType == "UPF" {
			upfIwkEpsIndFilter = bson.M{
				"upfinfo": bson.M{
					mongoOpElemMatch: bson.M{
						"iwkepsind": true,
					},
				},
			}
		}
		if queryParameters[queryParamUpfIwkEpsInd].negative {
			upfIwkEpsIndFilter = bson.M{
				"$not": upfIwkEpsIndFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), upfIwkEpsIndFilter)
	}

	// [Query-31] chf-supported-plmn
	if queryParameters[queryParamChfSupportedPlmn] != nil {
		var chfSupportedPlmnFilter bson.M
		chfSupportedPlmn := queryParameters[queryParamChfSupportedPlmn].value
		if targetNfType == "CHF" {
			chfSupportedPlmnFilter = bson.M{
				"$or": []bson.M{
					{
						"chfinfo": bson.M{
							mongoOpElemMatch: bson.M{
								"plmnrangelist": bson.M{
									mongoOpElemMatch: bson.M{
										"start": bson.M{
											"$lte": chfSupportedPlmn,
										},
										"end": bson.M{
											"$gte": chfSupportedPlmn,
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
				"$not": chfSupportedPlmnFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), chfSupportedPlmnFilter)
	}

	// [Query-32]  preferred-locality
	// TODO: if no match
	if queryParameters[queryParamPreferredLocality] != nil {
		preferredLocality := queryParameters[queryParamPreferredLocality].value
		preferredLocalityFilter := bson.M{
			"locality": preferredLocality,
		}
		if queryParameters[queryParamPreferredLocality].negative {
			preferredLocalityFilter = bson.M{
				"$not": preferredLocalityFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), preferredLocalityFilter)
	}

	// [Query-33] access-type
	if queryParameters[queryParamAccessType] != nil {
		accessType := queryParameters[queryParamAccessType].value
		accessTypeFilter := bson.M{
			"smfinfo": bson.M{
				mongoOpElemMatch: bson.M{
					"accesstype": accessType,
				},
			},
		}
		if queryParameters[queryParamAccessType].negative {
			accessTypeFilter = bson.M{
				"$not": accessTypeFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), accessTypeFilter)
	}

	// [Query-34] supported-features
	if queryParameters[queryParamSupportedFeatures] != nil {
		supportedFeatures := queryParameters[queryParamSupportedFeatures].value
		supportedFeaturesFilter := bson.M{
			"nfservices": bson.M{
				mongoOpElemMatch: bson.M{
					"supportedfeatures": supportedFeatures,
				},
			},
		}
		if queryParameters[queryParamSupportedFeatures].negative {
			supportedFeaturesFilter = bson.M{
				"$not": supportedFeaturesFilter,
			}
		}
		filter[logicalOperator] = append(filter[logicalOperator].([]bson.M), supportedFeaturesFilter)
	}

	return filter
}

func GetRequesterAndTargetNfTypeGivenQueryParameters(queryParameters url.Values) (requesterNfType, targetNfType string) {
	requesterNfType, targetNfType = "UNKNOWN_NF", "UNKNOWN_NF"
	if queryParameters[queryParamRequesterNFType] != nil {
		requesterNfType = fmt.Sprint(queryParameters[queryParamRequesterNFType][0])
	}
	if queryParameters[queryParamTargetNFType] != nil {
		targetNfType = fmt.Sprint(queryParameters[queryParamTargetNFType][0])
	}
	return requesterNfType, targetNfType
}
