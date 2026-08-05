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

	"github.com/omec-project/openapi/v2/models"
)

// Decode converts source (any []map[string]any or []any value) into []models.NFProfileDiscovery.
// format is retained for API compatibility but is no longer used.
func Decode(source any, _ string) ([]models.NFProfileDiscovery, error) {
	// json.Unmarshal uses pre-compiled field offsets (faster than mapstructure reflection).
	// Pre-process first to decode any JSON-string-encoded arrays/objects in the source.
	b, err := json.Marshal(preprocessJSONStrings(source))
	if err != nil {
		return nil, fmt.Errorf("marshal failed: %w", err)
	}
	var target []models.NFProfileDiscovery
	if err := json.Unmarshal(b, &target); err != nil {
		return nil, fmt.Errorf("unmarshal failed: %w", err)
	}
	for i, p := range target {
		if err := validateNFProfileDiscovery(p); err != nil {
			return nil, fmt.Errorf("profile[%d] violates TS 29.510 constraints: %w", i, err)
		}
	}
	return target, nil
}

// preprocessJSONStrings recursively copies v, decoding string values that contain
// a JSON array or object into their native Go types. This preserves backward
// compatibility with documents that stored structured fields as JSON strings.
// For Go struct/pointer/slice values (e.g. from unit tests), it converts them
// through a JSON round-trip and strips empty-string fields to avoid enum
// validation errors on the outer unmarshal (MongoDB data never stores empty
// strings for validated enum fields).
func preprocessJSONStrings(v any) any {
	switch val := v.(type) {
	case nil:
		return nil
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, elem := range val {
			out[k] = preprocessJSONStrings(elem)
		}
		return out
	case []map[string]any:
		out := make([]any, len(val))
		for i, elem := range val {
			out[i] = preprocessJSONStrings(elem)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, elem := range val {
			out[i] = preprocessJSONStrings(elem)
		}
		return out
	case string:
		if len(val) > 0 && (val[0] == '[' || val[0] == '{') {
			var parsed any
			if json.Unmarshal([]byte(val), &parsed) == nil {
				return parsed
			}
		}
		return val
	default:
		// Pass primitive types through without a JSON round-trip to avoid recursion.
		switch val.(type) {
		case bool,
			int, int8, int16, int32, int64,
			uint, uint8, uint16, uint32, uint64,
			float32, float64:
			return val
		}
		// For Go structs/pointers/slices: convert through JSON, then strip empty
		// strings so zero-value enum fields don't fail custom UnmarshalJSON.
		b, err := json.Marshal(val)
		if err != nil {
			return val
		}
		var intermediate any
		if json.Unmarshal(b, &intermediate) != nil {
			return val
		}
		return removeEmptyStrings(preprocessJSONStrings(intermediate))
	}
}

// removeEmptyStrings removes empty-string entries from maps/slices. Only called
// for data that came through the Go struct → JSON round-trip path.
func removeEmptyStrings(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for k, elem := range val {
			if s, ok := elem.(string); ok && s == "" {
				delete(val, k)
			} else {
				val[k] = removeEmptyStrings(elem)
			}
		}
		return val
	case []any:
		for i, elem := range val {
			val[i] = removeEmptyStrings(elem)
		}
		return val
	default:
		return val
	}
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
