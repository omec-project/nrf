// SPDX-FileCopyrightText: 2025 Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/google/uuid"
	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/nrf/polling"
	"github.com/omec-project/openapi/v2/models"
	"go.mongodb.org/mongo-driver/bson"
)

func NnrfNFManagementDataModel(nf *models.NFProfile, nfprofile models.NFProfile) error {
	if nfprofile.GetNfInstanceId() == "" {
		return fmt.Errorf("NfInstanceId field is required")
	}
	nf.SetNfInstanceId(nfprofile.GetNfInstanceId())

	if nfprofile.GetNfType() == "" {
		return fmt.Errorf("NfType field is required")
	}
	nf.SetNfType(nfprofile.GetNfType())

	if nfprofile.GetNfStatus() == "" {
		return fmt.Errorf("NfStatus field is required")
	}
	nf.SetNfStatus(nfprofile.GetNfStatus())

	plmnList, hasPlmnList := nfprofile.GetPlmnListOk()
	nfPlmnList, err := buildNfProfilePlmnList(plmnList, hasPlmnList)
	if err != nil {
		return err
	}

	nnrfNFManagementCondition(nf, nfprofile)
	nf.SetPlmnList(nfPlmnList)
	nnrfNFManagementOption(nf, nfprofile)

	return nil
}

func buildNfProfilePlmnList(nfProvidedPlmnList []models.PlmnId, hasProvidedPlmnList bool) ([]models.PlmnId, error) {
	// NF provided a list of supported PLMNs
	if hasProvidedPlmnList && len(nfProvidedPlmnList) != 0 {
		return nfProvidedPlmnList, nil
	}
	// NF did not provide supported PLMNs: fetch from webconsole
	logger.ManagementLog.Warnln("PLMN config not provided by NF, using supported PLMNs from webconsole")
	supportedPlmnList, err := polling.FetchPlmnConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PLMN config from webconsole: %v", err)
	}
	logger.ManagementLog.Debugf("Fetched PLMN list from webconsole: %+v", supportedPlmnList)
	if len(supportedPlmnList) == 0 {
		return nil, fmt.Errorf("PLMN config not provided by NF and no local PLMN config available")
	}
	return supportedPlmnList, nil
}

func SetsubscriptionId() string {
	id, err := uuid.NewRandom()
	if err != nil {
		logger.ManagementLog.Errorf("failed to generate UUID for subscription ID: %v", err)
		// Fallback to a time-based ID to avoid panicking and keep subscription creation non-fatal
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return id.String()
}

func nnrfNFManagementCondition(nf *models.NFProfile, nfprofile models.NFProfile) {
	// HeartBeatTimer
	if !factory.NrfConfig.Configuration.NfProfileExpiryEnable {
		// setting 1day keepAliveTimer value
		factory.NrfConfig.Configuration.NfKeepAliveTime = 24 * 60 * 60
	} else if factory.NrfConfig.Configuration.NfKeepAliveTime == 0 {
		logger.ManagementLog.Infoln("NfProfileExpiryEnable: true but keepAliveTime: 0, setting default keepAliveTimer: 60 sec")
		factory.NrfConfig.Configuration.NfKeepAliveTime = 60
	}
	nf.SetHeartBeatTimer(factory.NrfConfig.Configuration.NfKeepAliveTime)
	logger.ManagementLog.Infof("heartbeat timer value: %d sec", nf.GetHeartBeatTimer())

	// fqdn
	if fqdn, ok := nfprofile.GetFqdnOk(); ok {
		nf.SetFqdn(*fqdn)
	}
	// interPlmnFqdn
	interPlmnFqdn, ok := nfprofile.GetInterPlmnFqdnOk()
	if ok {
		nf.SetInterPlmnFqdn(*interPlmnFqdn)
	}
	// ipv4Addresses
	if ipv4Addresses, ok := nfprofile.GetIpv4AddressesOk(); ok {
		a := make([]string, len(ipv4Addresses))
		copy(a, ipv4Addresses)
		nf.SetIpv4Addresses(a)
	}
	// ipv6Addresses
	if ipv6Addresses, ok := nfprofile.GetIpv6AddressesOk(); ok {
		a := make([]string, len(ipv6Addresses))
		copy(a, ipv6Addresses)
		nf.SetIpv6Addresses(a)
	}
}

func nnrfNFManagementOption(nf *models.NFProfile, nfprofile models.NFProfile) {
	// sNssais
	if sNssais, ok := nfprofile.GetSNssaisOk(); ok {
		a := make([]models.Snssai, len(sNssais))
		copy(a, sNssais)
		nf.SetSNssais(a)
	}

	// nsiList
	if nsiList, ok := nfprofile.GetNsiListOk(); ok {
		a := make([]string, len(nsiList))
		copy(a, nsiList)
		nf.SetNsiList(a)
	}

	// allowedPlmns
	if allowedPlmns, ok := nfprofile.GetAllowedPlmnsOk(); ok {
		a := make([]models.PlmnId, len(allowedPlmns))
		copy(a, allowedPlmns)
		nf.SetAllowedPlmns(a)
	}

	// allowedNfTypes
	if allowedNfTypes, ok := nfprofile.GetAllowedNfTypesOk(); ok {
		a := make([]models.NFType, len(allowedNfTypes))
		copy(a, allowedNfTypes)
		nf.SetAllowedNfTypes(a)
	}
	// allowedNfDomains
	if allowedNfDomains, ok := nfprofile.GetAllowedNfDomainsOk(); ok {
		a := make([]string, len(allowedNfDomains))
		copy(a, allowedNfDomains)
		nf.SetAllowedNfDomains(a)
	}

	// allowedNssais
	if allowedNssais, ok := nfprofile.GetAllowedNssaisOk(); ok {
		a := make([]models.Snssai, len(allowedNssais))
		copy(a, allowedNssais)
		nf.SetAllowedNssais(a)
	}
	// Priority
	if nfprofile.GetPriority() > 0 && nfprofile.GetPriority() <= 65535 {
		nf.SetPriority(nfprofile.GetPriority())
	}
	// Capacity
	if nfprofile.GetCapacity() > 0 && nfprofile.GetCapacity() <= 65535 {
		nf.SetCapacity(nfprofile.GetCapacity())
	}
	// Load
	if nfprofile.GetLoad() > 0 && nfprofile.GetLoad() <= 100 {
		nf.SetLoad(nfprofile.GetLoad())
	}
	// Locality
	if nfprofile.GetLocality() != "" {
		nf.SetLocality(nfprofile.GetLocality())
	}

	// udrInfo
	if nfprofile.UdrInfo != nil {
		a := models.NewUdrInfo()

		if groupId, ok := nfprofile.UdrInfo.GetGroupIdOk(); ok {
			a.SetGroupId(*groupId)
		}

		if supiRanges, ok := nfprofile.UdrInfo.GetSupiRangesOk(); ok {
			a.SetSupiRanges(supiRanges)
		}

		if gpsiRanges, ok := nfprofile.UdrInfo.GetGpsiRangesOk(); ok {
			a.SetGpsiRanges(gpsiRanges)
		}

		if externalGroupIdentifiersRanges, ok := nfprofile.UdrInfo.GetExternalGroupIdentifiersRangesOk(); ok {
			a.SetExternalGroupIdentifiersRanges(externalGroupIdentifiersRanges)
		}

		if supportedDataSets, ok := nfprofile.UdrInfo.GetSupportedDataSetsOk(); ok {
			a.SetSupportedDataSets(supportedDataSets)
		}

		nf.SetUdrInfo(*a)
	}
	// udmInfo
	if nfprofile.UdmInfo != nil {
		a := models.NewUdmInfo()

		if groupId, ok := nfprofile.UdmInfo.GetGroupIdOk(); ok {
			a.SetGroupId(*groupId)
		}

		if supiRanges, ok := nfprofile.UdmInfo.GetSupiRangesOk(); ok {
			a.SetSupiRanges(supiRanges)
		}

		if gpsiRanges, ok := nfprofile.UdmInfo.GetGpsiRangesOk(); ok {
			a.SetGpsiRanges(gpsiRanges)
		}

		if externalGroupIdentifiersRanges, ok := nfprofile.UdmInfo.GetExternalGroupIdentifiersRangesOk(); ok {
			a.SetExternalGroupIdentifiersRanges(externalGroupIdentifiersRanges)
		}

		if routingIndicators, ok := nfprofile.UdmInfo.GetRoutingIndicatorsOk(); ok {
			a.SetRoutingIndicators(routingIndicators)
		}

		nf.SetUdmInfo(*a)
	}
	// ausfInfo
	if nfprofile.AusfInfo != nil {
		a := models.NewAusfInfo()

		if groupId, ok := nfprofile.AusfInfo.GetGroupIdOk(); ok {
			a.SetGroupId(*groupId)
		}

		if supiRanges, ok := nfprofile.AusfInfo.GetSupiRangesOk(); ok {
			a.SetSupiRanges(supiRanges)
		}

		if routingIndicators, ok := nfprofile.AusfInfo.GetRoutingIndicatorsOk(); ok {
			a.SetRoutingIndicators(routingIndicators)
		}

		nf.SetAusfInfo(*a)
	}
	// amfInfo
	if nfprofile.AmfInfo != nil {
		a := models.NewAmfInfoWithDefaults()

		if amfSetId, ok := nfprofile.AmfInfo.GetAmfSetIdOk(); ok {
			a.SetAmfSetId(*amfSetId)
		}

		if amfRegionId, ok := nfprofile.AmfInfo.GetAmfRegionIdOk(); ok {
			a.SetAmfRegionId(*amfRegionId)
		}

		if guamiList, ok := nfprofile.AmfInfo.GetGuamiListOk(); ok {
			a.SetGuamiList(guamiList)
		}

		if taiList, ok := nfprofile.AmfInfo.GetTaiListOk(); ok {
			a.SetTaiList(taiList)
		}

		if taiRangeList, ok := nfprofile.AmfInfo.GetTaiRangeListOk(); ok {
			a.SetTaiRangeList(taiRangeList)
		}

		if backupInfoAmfFailure, ok := nfprofile.AmfInfo.GetBackupInfoAmfFailureOk(); ok {
			a.SetBackupInfoAmfFailure(backupInfoAmfFailure)
		}

		if backupInfoAmfRemoval, ok := nfprofile.AmfInfo.GetBackupInfoAmfRemovalOk(); ok {
			a.SetBackupInfoAmfRemoval(backupInfoAmfRemoval)
		}

		if nfprofile.AmfInfo.N2InterfaceAmfInfo.IsSet() {
			a.N2InterfaceAmfInfo = nfprofile.AmfInfo.N2InterfaceAmfInfo
		}
		nf.SetAmfInfo(*a)
	}
	// smfInfo
	if nfprofile.SmfInfo != nil {
		a := models.NewSmfInfoWithDefaults()

		if sNssaiSmfInfoList, ok := nfprofile.SmfInfo.GetSNssaiSmfInfoListOk(); ok {
			a.SetSNssaiSmfInfoList(sNssaiSmfInfoList)
		}

		if taiList, ok := nfprofile.SmfInfo.GetTaiListOk(); ok {
			a.SetTaiList(taiList)
		}

		if taiRangeList, ok := nfprofile.SmfInfo.GetTaiRangeListOk(); ok {
			a.SetTaiRangeList(taiRangeList)
		}

		if pgwFqdn, ok := nfprofile.SmfInfo.GetPgwFqdnOk(); ok {
			a.SetPgwFqdn(*pgwFqdn)
		}

		if accessType, ok := nfprofile.SmfInfo.GetAccessTypeOk(); ok {
			a.SetAccessType(accessType)
		}
		nf.SetSmfInfo(*a)
	}
	// upfInfo
	if nfprofile.UpfInfo != nil {
		a := models.NewUpfInfoWithDefaults()

		if sNssaiUpfInfoList, ok := nfprofile.UpfInfo.GetSNssaiUpfInfoListOk(); ok {
			a.SetSNssaiUpfInfoList(sNssaiUpfInfoList)
		}

		if smfServingArea, ok := nfprofile.UpfInfo.GetSmfServingAreaOk(); ok {
			a.SetSmfServingArea(smfServingArea)
		}

		if interfaceUpfInfoList, ok := nfprofile.UpfInfo.GetInterfaceUpfInfoListOk(); ok {
			a.SetInterfaceUpfInfoList(interfaceUpfInfoList)
		}

		a.SetIwkEpsInd(nfprofile.UpfInfo.GetIwkEpsInd())

		nf.SetUpfInfo(*a)
	}
	// pcfInfo
	if nfprofile.PcfInfo != nil {
		a := models.NewPcfInfo()

		if dnnList, ok := nfprofile.PcfInfo.GetDnnListOk(); ok {
			a.SetDnnList(dnnList)
		}

		if supiRanges, ok := nfprofile.PcfInfo.GetSupiRangesOk(); ok {
			a.SetSupiRanges(supiRanges)
		}

		if rxDiamHost, ok := nfprofile.PcfInfo.GetRxDiamHostOk(); ok {
			a.SetRxDiamHost(*rxDiamHost)
		}

		if rxDiamRealm, ok := nfprofile.PcfInfo.GetRxDiamRealmOk(); ok {
			a.SetRxDiamRealm(*rxDiamRealm)
		}
		nf.SetPcfInfo(*a)
	}
	// bsfInfo
	if nfprofile.BsfInfo != nil {
		a := models.NewBsfInfo()

		if dnnList, ok := nfprofile.BsfInfo.GetDnnListOk(); ok {
			a.SetDnnList(dnnList)
		}

		if ipDomainList, ok := nfprofile.BsfInfo.GetIpDomainListOk(); ok {
			a.SetIpDomainList(ipDomainList)
		}

		if ipv4AddressRanges, ok := nfprofile.BsfInfo.GetIpv4AddressRangesOk(); ok {
			b := make([]models.Ipv4AddressRange, len(ipv4AddressRanges))
			for i, rang := range ipv4AddressRanges {
				b[i].SetStart(strconv.FormatInt(Ipv4ToInt(rang.GetStart()), 10))
				b[i].SetEnd(strconv.FormatInt(Ipv4ToInt(rang.GetEnd()), 10))
			}
			a.SetIpv4AddressRanges(b)
		}

		if ipv6PrefixRanges, ok := nfprofile.BsfInfo.GetIpv6PrefixRangesOk(); ok {
			b := make([]models.Ipv6PrefixRange, len(ipv6PrefixRanges))
			for i, rang := range ipv6PrefixRanges {
				b[i].SetStart(Ipv6ToInt(rang.GetStart()).String())
				b[i].SetEnd(Ipv6ToInt(rang.GetEnd()).String())
			}
			a.SetIpv6PrefixRanges(b)
		}
		nf.SetBsfInfo(*a)
	}
	// chfInfo
	if chfInfo, ok := nfprofile.GetChfInfoOk(); ok {
		a := models.NewChfInfo()

		if supiRangeList, ok := chfInfo.GetSupiRangeListOk(); ok {
			a.SetSupiRangeList(supiRangeList)
		}

		if gpsiRangeList, ok := chfInfo.GetGpsiRangeListOk(); ok {
			a.SetGpsiRangeList(gpsiRangeList)
		}

		if plmnRangeList, ok := chfInfo.GetPlmnRangeListOk(); ok {
			a.SetPlmnRangeList(plmnRangeList)
		}
		nf.SetChfInfo(*a)
	}
	// nrfInfo
	if nrfInfo, ok := nfprofile.GetNrfInfoOk(); ok {
		nf.SetNrfInfo(*nrfInfo)
	}

	// recoveryTime
	if recoveryTime, ok := nfprofile.GetRecoveryTimeOk(); ok {
		// Update when restart (Setting by NF itself)
		nf.SetRecoveryTime(*recoveryTime)
	}

	// nfServicePersistence
	nf.SetNfServicePersistence(nfprofile.GetNfServicePersistence())

	// nfServices
	if nfServices, ok := nfprofile.GetNfServicesOk(); ok {
		a := make([]models.NFService, len(nfServices))
		copy(a, nfServices)
		nf.SetNfServices(a)
	}
}

func GetNfInstanceURI(nfInstID string) string {
	return factory.NrfConfig.GetSbiUri() + "/nnrf-nfm/v1/nf-instances/" + nfInstID
}

func SetLocationHeader(nfprofile models.NFProfile) string {
	var modifyUL UriList
	var locationHeader []string

	// set nfprofile location
	locationHeader = append(locationHeader, GetNfInstanceURI(nfprofile.GetNfInstanceId()))

	collName := "urilist"
	nfType := nfprofile.NfType
	filter := bson.M{"nfType": nfType}

	ul, _ := dbadapter.DBClient.RestfulAPIGetOne(collName, filter)

	var originalUL UriList
	err := mapstructure.Decode(ul, &originalUL)
	if err != nil {
		panic(err)
	}

	// obtain location header = NF URI
	nnrfUriList(&originalUL, &modifyUL, locationHeader)
	modifyUL.NfType = nfprofile.NfType

	tmp, err := json.Marshal(modifyUL)
	if err != nil {
		logger.ManagementLog.Error(err)
	}
	putData := bson.M{}
	err = json.Unmarshal(tmp, &putData)
	if err != nil {
		logger.ManagementLog.Error(err)
	}

	if ok, _ := dbadapter.DBClient.RestfulAPIPutOne(collName, filter, putData); ok {
		logger.ManagementLog.Info("urilist update")
	} else {
		logger.ManagementLog.Info("urilist create")
	}

	return locationHeader[0]
}

func setUriListByFilter(filter bson.M, uriList *[]string) {
	filterNfTypeResultsRaw, _ := dbadapter.DBClient.RestfulAPIGetMany("Subscriptions", filter)
	var filterNfTypeResults []models.SubscriptionData
	stringToDateTimeHook := func(
		f reflect.Type,
		t reflect.Type,
		data any,
	) (any, error) {
		if t == reflect.TypeOf(time.Time{}) && f == reflect.TypeOf("") {
			return time.Parse(time.RFC3339, data.(string))
		}
		return data, nil
	}

	config := mapstructure.DecoderConfig{
		DecodeHook: stringToDateTimeHook,
		Result:     &filterNfTypeResults,
	}

	decoder, err := mapstructure.NewDecoder(&config)
	if err != nil {
		logger.ManagementLog.Errorf("converter setup failed: %v", err)
		return
	}

	err = decoder.Decode(filterNfTypeResultsRaw)
	if err != nil {
		logger.ManagementLog.Error(err)
	}

	for _, subscr := range filterNfTypeResults {
		*uriList = append(*uriList, subscr.GetNfStatusNotificationUri())
	}
}

func nnrfUriList(originalUL *UriList, UL *UriList, location []string) {
	var b *Links
	var flag bool
	var c []models.Link
	flag = true
	b = new(Links)
	items := originalUL.Link.Item
	size := len(location) + len(items)

	// check duplicate
	for _, item := range items {
		if item.GetHref() == location[0] {
			flag = false
			break
		}
	}

	if flag {
		c = make([]models.Link, size)
		copy(c, items)
		for i, loc := range location {
			c[len(items)+i].SetHref(loc)
		}
	} else {
		c = make([]models.Link, size-1)
		copy(c, items)
	}

	b.Item = c
	UL.Link = *b
}

func GetNotificationUri(nfProfile models.NFProfile) []string {
	var uriList []string

	// nfTypeCond
	nfTypeCond := bson.M{
		"subscrCond": bson.M{
			"nfType": nfProfile.GetNfType(),
		},
	}
	setUriListByFilter(nfTypeCond, &uriList)

	// NfInstanceIdCond
	nfInstanceIDCond := bson.M{
		"subscrCond": bson.M{
			"nfInstanceId": nfProfile.GetNfInstanceId(),
		},
	}
	setUriListByFilter(nfInstanceIDCond, &uriList)

	// ServiceNameCond
	if nfServices, ok := nfProfile.GetNfServicesOk(); ok {
		var ServiceNameCond bson.M
		var serviceNames bson.A
		for _, nfService := range nfServices {
			serviceNames = append(serviceNames, string(nfService.ServiceName))
		}
		ServiceNameCond = bson.M{
			"subscrCond.serviceName": bson.M{
				"$in": serviceNames,
			},
		}
		setUriListByFilter(ServiceNameCond, &uriList)
	}

	// AmfCond
	if amfInfo, ok := nfProfile.GetAmfInfoOk(); ok {
		amfCond := bson.M{
			"subscrCond": bson.M{
				"amfSetId":    amfInfo.GetAmfSetId(),
				"amfRegionId": amfInfo.GetAmfRegionId(),
			},
		}
		setUriListByFilter(amfCond, &uriList)

		var guamiListFilter bson.M
		if guamiList, ok := amfInfo.GetGuamiListOk(); ok {
			var guamiListBsonArray bson.A
			for _, guami := range guamiList {
				tmp, err := json.Marshal(guami)
				if err != nil {
					logger.ManagementLog.Error(err)
				}
				guamiMarshal := bson.M{}
				err = json.Unmarshal(tmp, &guamiMarshal)
				if err != nil {
					logger.ManagementLog.Error(err)
				}

				guamiListBsonArray = append(guamiListBsonArray, bson.M{"subscrCond": bson.M{"$elemMatch": guamiMarshal}})
			}
			guamiListFilter = bson.M{
				"$or": guamiListBsonArray,
			}
			setUriListByFilter(guamiListFilter, &uriList)
		}
	}

	// NetworkSliceCond
	if sNssais, ok := nfProfile.GetSNssaisOk(); ok {
		var networkSliceFilter bson.M
		var snssaisBsonArray bson.A
		for _, snssai := range sNssais {
			tmp, err := json.Marshal(snssai)
			if err != nil {
				logger.ManagementLog.Error(err)
			}
			snssaiMarshal := bson.M{}
			err = json.Unmarshal(tmp, &snssaiMarshal)
			if err != nil {
				logger.ManagementLog.Error(err)
			}

			snssaisBsonArray = append(snssaisBsonArray, bson.M{"subscrCond": bson.M{"$elemMatch": snssaiMarshal}})
		}

		var nsiListBsonArray bson.A
		if nsiList, ok := nfProfile.GetNsiListOk(); ok {
			for _, nsi := range nsiList {
				nsiListBsonArray = append(nsiListBsonArray, nsi)
			}
		}

		if nsiListBsonArray != nil {
			networkSliceFilter = bson.M{
				"$and": bson.A{
					bson.M{
						"subscrCond.nsiList": bson.M{
							"$in": nsiListBsonArray,
						},
					},
					bson.M{
						"$or": snssaisBsonArray,
					},
				},
			}
		} else {
			networkSliceFilter = bson.M{
				"$and": bson.A{
					bson.M{
						"$or": snssaisBsonArray,
					},
				},
			}
		}
		setUriListByFilter(networkSliceFilter, &uriList)
	}

	// NfGroupCond
	nfType := nfProfile.GetNfType()
	udrInfo, okUdr := nfProfile.GetUdrInfoOk()
	udmInfo, okUdm := nfProfile.GetUdmInfoOk()
	ausfInfo, okAusf := nfProfile.GetAusfInfoOk()
	switch {
	case okUdr:
		nfGroupCond := bson.M{
			"subscrCond": bson.M{
				"nfType":    nfType,
				"nfGroupId": udrInfo.GetGroupId(),
			},
		}
		setUriListByFilter(nfGroupCond, &uriList)
	case okUdm:
		nfGroupCond := bson.M{
			"subscrCond": bson.M{
				"nfType":    nfType,
				"nfGroupId": udmInfo.GetGroupId(),
			},
		}
		setUriListByFilter(nfGroupCond, &uriList)
	case okAusf:
		nfGroupCond := bson.M{
			"subscrCond": bson.M{
				"nfType":    nfType,
				"nfGroupId": ausfInfo.GetGroupId(),
			},
		}
		setUriListByFilter(nfGroupCond, &uriList)
	}

	return uriList
}

func NnrfUriListLimit(originalUL *UriList, limit int) {
	// response limit

	if limit < len(originalUL.Link.Item) {
		b := new(Links)
		c := make([]models.Link, limit)
		copy(c, originalUL.Link.Item[:limit])
		b.Item = c
		originalUL.Link = *b
	}
}
