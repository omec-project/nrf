// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

//go:build !debug
// +build !debug

package util

import (
	"reflect"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/omec-project/openapi/v2/models"
)

// Decode - Only support []map[string]interface to []models.NFProfileDiscovery
func Decode(source any, format string) ([]models.NFProfileDiscovery, error) {
	var target []models.NFProfileDiscovery

	// config mapstruct
	stringToDateTimeHook := func(
		f reflect.Type,
		t reflect.Type,
		data any,
	) (any, error) {
		if t == reflect.TypeOf(time.Time{}) && f == reflect.TypeOf("") {
			return time.Parse(format, data.(string))
		}
		return data, nil
	}

	config := mapstructure.DecoderConfig{
		DecodeHook: stringToDateTimeHook,
		Result:     &target,
	}

	decoder, err := mapstructure.NewDecoder(&config)
	if err != nil {
		return nil, err
	}

	// Decode result to NfProfile structure
	err = decoder.Decode(source)
	if err != nil {
		return nil, err
	}
	return target, nil
}

func ConvertNFProfileDiscoveryToNFProfile(discovery models.NFProfileDiscovery) models.NFProfile {
	return models.NFProfile{
		NfInstanceId:                     discovery.NfInstanceId,
		NfInstanceName:                   discovery.NfInstanceName,
		NfType:                           discovery.NfType,
		NfStatus:                         discovery.NfStatus,
		CollocatedNfInstances:            discovery.CollocatedNfInstances,
		PlmnList:                         discovery.PlmnList,
		SnpnList:                         discovery.SnpnList,
		SNssais:                          discovery.SNssais,
		PerPlmnSnssaiList:                discovery.PerPlmnSnssaiList,
		NsiList:                          discovery.NsiList,
		Fqdn:                             discovery.Fqdn,
		InterPlmnFqdn:                    discovery.InterPlmnFqdn,
		Ipv4Addresses:                    discovery.Ipv4Addresses,
		Ipv6Addresses:                    discovery.Ipv6Addresses,
		AllowedPlmns:                     discovery.AllowedPlmns,
		AllowedSnpns:                     discovery.AllowedSnpns,
		AllowedNfTypes:                   discovery.AllowedNfTypes,
		AllowedNfDomains:                 discovery.AllowedNfDomains,
		AllowedNssais:                    discovery.AllowedNssais,
		AllowedRuleSet:                   discovery.AllowedRuleSet,
		Priority:                         discovery.Priority,
		Capacity:                         discovery.Capacity,
		Load:                             discovery.Load,
		LoadTimeStamp:                    discovery.LoadTimeStamp,
		Locality:                         discovery.Locality,
		ExtLocality:                      discovery.ExtLocality,
		UdrInfo:                          discovery.UdrInfo,
		UdrInfoList:                      discovery.UdrInfoList,
		UdmInfo:                          discovery.UdmInfo,
		UdmInfoList:                      discovery.UdmInfoList,
		AusfInfo:                         discovery.AusfInfo,
		AusfInfoList:                     discovery.AusfInfoList,
		AmfInfo:                          discovery.AmfInfo,
		AmfInfoList:                      discovery.AmfInfoList,
		SmfInfo:                          discovery.SmfInfo,
		SmfInfoList:                      discovery.SmfInfoList,
		UpfInfo:                          discovery.UpfInfo,
		UpfInfoList:                      discovery.UpfInfoList,
		PcfInfo:                          discovery.PcfInfo,
		PcfInfoList:                      discovery.PcfInfoList,
		BsfInfo:                          discovery.BsfInfo,
		BsfInfoList:                      discovery.BsfInfoList,
		ChfInfo:                          discovery.ChfInfo,
		ChfInfoList:                      discovery.ChfInfoList,
		NefInfo:                          discovery.NefInfo,
		UdsfInfo:                         discovery.UdsfInfo,
		UdsfInfoList:                     discovery.UdsfInfoList,
		NwdafInfo:                        discovery.NwdafInfo,
		NwdafInfoList:                    discovery.NwdafInfoList,
		PcscfInfoList:                    discovery.PcscfInfoList,
		HssInfoList:                      discovery.HssInfoList,
		CustomInfo:                       discovery.CustomInfo,
		RecoveryTime:                     discovery.RecoveryTime,
		NfServicePersistence:             discovery.NfServicePersistence,
		NfServices:                       discovery.NfServices,
		NfServiceList:                    discovery.NfServiceList,
		DefaultNotificationSubscriptions: discovery.DefaultNotificationSubscriptions,
		LmfInfo:                          discovery.LmfInfo,
		GmlcInfo:                         discovery.GmlcInfo,
		NfSetIdList:                      discovery.NfSetIdList,
		ServingScope:                     discovery.ServingScope,
		LcHSupportInd:                    discovery.LcHSupportInd,
		OlcHSupportInd:                   discovery.OlcHSupportInd,
		NfSetRecoveryTimeList:            discovery.NfSetRecoveryTimeList,
		ServiceSetRecoveryTimeList:       discovery.ServiceSetRecoveryTimeList,
		ScpDomains:                       discovery.ScpDomains,
		ScpInfo:                          discovery.ScpInfo,
		SeppInfo:                         discovery.SeppInfo,
		VendorId:                         discovery.VendorId,
		SupportedVendorSpecificFeatures:  discovery.SupportedVendorSpecificFeatures,
		AanfInfoList:                     discovery.AanfInfoList,
		MfafInfo:                         discovery.MfafInfo,
		EasdfInfoList:                    discovery.EasdfInfoList,
		DccfInfo:                         discovery.DccfInfo,
		NsacfInfoList:                    discovery.NsacfInfoList,
		MbSmfInfoList:                    discovery.MbSmfInfoList,
		TsctsfInfoList:                   discovery.TsctsfInfoList,
		MbUpfInfoList:                    discovery.MbUpfInfoList,
		TrustAfInfo:                      discovery.TrustAfInfo,
		NssaafInfo:                       discovery.NssaafInfo,
		HniList:                          discovery.HniList,
		IwmscInfo:                        discovery.IwmscInfo,
		MnpfInfo:                         discovery.MnpfInfo,
		SmsfInfo:                         discovery.SmsfInfo,
		DcsfInfoList:                     discovery.DcsfInfoList,
		MrfInfoList:                      discovery.MrfInfoList,
		MrfpInfoList:                     discovery.MrfpInfoList,
		MfInfoList:                       discovery.MfInfoList,
		AdrfInfoList:                     discovery.AdrfInfoList,
		SelectionConditions:              discovery.SelectionConditions,
		CanaryRelease:                    discovery.CanaryRelease,
		ExclusiveCanaryReleaseSelection:  discovery.ExclusiveCanaryReleaseSelection,
		SharedProfileDataId:              discovery.SharedProfileDataId,
	}
}
