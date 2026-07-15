// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

//go:build !debug
// +build !debug

package util

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/omec-project/openapi/v2/models"
)

// Decode converts source (any []map[string]any or []any value) into []models.NFProfileDiscovery.
// format is the time layout used for time.Time fields (e.g. time.RFC3339).
func Decode(source any, format string) ([]models.NFProfileDiscovery, error) {
	var target []models.NFProfileDiscovery

	// Enhanced decode hook to handle various data types including JSON strings
	decodeHook := mapstructure.ComposeDecodeHookFunc(
		// Handle time parsing
		func(f reflect.Type, t reflect.Type, data any) (any, error) {
			if t == reflect.TypeFor[time.Time]() && f == reflect.TypeFor[string]() {
				return time.Parse(format, data.(string))
			}
			return data, nil
		},

		// Handle JSON string to slice/map conversion
		func(f reflect.Type, t reflect.Type, data any) (any, error) {
			if f == nil || t == nil || f.Kind() != reflect.String {
				return data, nil
			}
			str, ok := data.(string)
			if !ok {
				return data, nil
			}
			// Unwrap one level of pointer to reach the effective target kind.
			effectiveType := t
			isPtr := t.Kind() == reflect.Ptr
			if isPtr {
				effectiveType = t.Elem()
			}
			if effectiveType.Kind() != reflect.Slice && effectiveType.Kind() != reflect.Map {
				return data, nil
			}
			ptr := reflect.New(effectiveType)
			if err := json.Unmarshal([]byte(str), ptr.Interface()); err != nil {
				return nil, fmt.Errorf("invalid JSON string: %w", err)
			}
			if isPtr {
				return ptr.Interface(), nil
			}
			return ptr.Elem().Interface(), nil
		},
	)

	config := mapstructure.DecoderConfig{
		DecodeHook:       decodeHook,
		Result:           &target,
		WeaklyTypedInput: true,
	}

	decoder, err := mapstructure.NewDecoder(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to create decoder: %w", err)
	}

	err = decoder.Decode(source)
	if err != nil {
		return nil, fmt.Errorf("decoding failed: %w", err)
	}
	for i, p := range target {
		if err := validateNFProfileDiscovery(p); err != nil {
			return nil, fmt.Errorf("profile[%d] violates TS 29.510 constraints: %w", i, err)
		}
	}
	return target, nil
}

// validateNFProfileDiscovery enforces the numeric range constraints defined in
// TS 29.510 clause 6.1.6.2.2 (NFProfile) and TS 23.501 clause 5.15.2 (S-NSSAI)
// that mapstructure's WeaklyTypedInput coercion would otherwise silently bypass.
func validateNFProfileDiscovery(p models.NFProfileDiscovery) error {
	if priority, ok := p.GetPriorityOk(); ok && (*priority < 0 || *priority > 65535) {
		return fmt.Errorf("priority %d out of range [0, 65535]", *priority)
	}
	if capacity, ok := p.GetCapacityOk(); ok && (*capacity < 0 || *capacity > 65535) {
		return fmt.Errorf("capacity %d out of range [0, 65535]", *capacity)
	}
	if load, ok := p.GetLoadOk(); ok && (*load < 0 || *load > 100) {
		return fmt.Errorf("load %d out of range [0, 100]", *load)
	}
	for i, snssai := range p.SNssais {
		if snssai.GetSst() < 0 || snssai.GetSst() > 255 {
			return fmt.Errorf("sNssais[%d].sst %d out of range [0, 255]", i, snssai.GetSst())
		}
	}
	for i, snssai := range p.AllowedNssais {
		if snssai.GetSst() < 0 || snssai.GetSst() > 255 {
			return fmt.Errorf("allowedNssais[%d].sst %d out of range [0, 255]", i, snssai.GetSst())
		}
	}
	return nil
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
