// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

//go:build !debug
// +build !debug

package util

import (
	"testing"
	"time"

	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

func TestDecode(t *testing.T) {
	// Set time
	date := time.Now()
	dateFormat, _ := time.Parse(time.RFC3339, date.Format(time.RFC3339))

	testData1 := map[string]any{
		"NfInstanceId":   "0",
		"NfType":         models.NFTYPE_NRF,
		"NfStatus":       models.NFSTATUS_REGISTERED,
		"HeartBeatTimer": 10,
		"PlmnList": &[]models.PlmnId{ // Pattern: '^[0-9]{3}[0-9]{2,3}$'
			{
				Mcc: "111",
				Mnc: "111",
			},
		},
		"SNssais": &[]models.Snssai{ // range 0-255
			{
				Sst: 1, // eMBB per TS 23.501
				Sd:  openapi.PtrString("010203"),
			},
		},
		"NsiList": []string{
			"nsi0",
		},
		"Fqdn":          "fqdn",
		"InterPlmnFqdn": "InterPlmnFqdn",
		"Ipv4Addresses": []string{
			"140.113.1.1",
		},
		"Ipv6Addresses": []string{
			"fc00::",
		},
		"AllowedPlmns": &[]models.PlmnId{
			{
				Mcc: "111",
				Mnc: "111",
			},
		},
		"AllowedNfTypes": []models.NFType{
			models.NFTYPE_NRF,
		},
		"AllowedNfDomains": []string{
			"nfdomain1",
		},
		"AllowedNssais": &[]models.Snssai{
			{
				Sst: 2, // URLLC per TS 23.501
				Sd:  openapi.PtrString("040506"),
			},
		},
		"Priority":             1,
		"Capacity":             1,
		"Load":                 1,
		"Locality":             "NCTU",
		"UdrInfo":              &models.UdrInfo{},
		"UdmInfo":              &models.UdmInfo{},
		"AusfInfo":             &models.AusfInfo{},
		"AmfInfo":              &models.AmfInfo{},
		"SmfInfo":              &models.SmfInfo{},
		"UpfInfo":              &models.UpfInfo{},
		"PcfInfo":              &models.PcfInfo{},
		"BsfInfo":              &models.BsfInfo{},
		"ChfInfo":              &models.ChfInfo{},
		"NrfInfo":              &models.NrfInfo{},
		"CustomInfo":           &map[string]any{},
		"RecoveryTime":         &dateFormat,
		"NfServicePersistence": true,
		"NfServices": &[]models.NFService{
			{
				ServiceName:     models.SERVICENAME_NNRF_DISC,
				NfServiceStatus: models.NFSERVICESTATUS_REGISTERED,
				AllowedNfDomains: []string{
					"nfdomain3",
					"nfdomain4",
				},
			},
		},
	}

	source := []map[string]any{
		testData1,
	}

	target, err := Decode(source, time.RFC3339)
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if len(target) != 1 {
		t.Fatalf("expected 1 decoded profile, got %d", len(target))
	}

	profile := target[0]
	if profile.GetNfInstanceId() != "0" {
		t.Fatalf("unexpected NF instance ID: %q", profile.GetNfInstanceId())
	}
	if profile.GetNfType() != models.NFTYPE_NRF {
		t.Fatalf("unexpected NF type: %q", profile.GetNfType())
	}
	services, ok := profile.GetNfServicesOk()
	if !ok || len(services) != 1 {
		t.Fatalf("expected 1 decoded NF service, got %v", services)
	}
	if services[0].GetServiceName() != models.SERVICENAME_NNRF_DISC {
		t.Fatalf("unexpected service name: %q", services[0].GetServiceName())
	}

	t.Logf("%+v", target)
}

// TestDecodeRangeValidation verifies that validateNFProfileDiscovery rejects
// values that fall outside the numeric ranges mandated by TS 29.510 clause
// 6.1.6.2.2 (NFProfile) and TS 23.501 clause 5.15.2 (S-NSSAI).
func TestDecodeRangeValidation(t *testing.T) {
	baseProfile := map[string]any{
		"NfInstanceId": "range-test",
		"NfType":       models.NFTYPE_NRF,
		"NfStatus":     models.NFSTATUS_REGISTERED,
	}

	cases := []struct {
		name    string
		field   string
		value   any
		wantErr bool
	}{
		// AllowedNssais.Sst: must be in [0, 255] per TS 23.501
		{
			name:    "AllowedNssais Sst above max (333)",
			field:   "AllowedNssais",
			value:   &[]models.Snssai{{Sst: 333, Sd: openapi.PtrString("000001")}},
			wantErr: true,
		},
		{
			name:    "AllowedNssais Sst at max (255)",
			field:   "AllowedNssais",
			value:   &[]models.Snssai{{Sst: 255}},
			wantErr: false,
		},
		// Capacity: must be in [0, 65535] per TS 29.510
		{
			name:    "Capacity above max (70000)",
			field:   "Capacity",
			value:   int32(70000),
			wantErr: true,
		},
		{
			name:    "Capacity at max (65535)",
			field:   "Capacity",
			value:   int32(65535),
			wantErr: false,
		},
		{
			name:    "Capacity at min (0)",
			field:   "Capacity",
			value:   int32(0),
			wantErr: false,
		},
		// Load: must be in [0, 100] per TS 29.510
		{
			name:    "Load above max (101)",
			field:   "Load",
			value:   int32(101),
			wantErr: true,
		},
		{
			name:    "Load at max (100)",
			field:   "Load",
			value:   int32(100),
			wantErr: false,
		},
		// Priority: must be in [0, 65535] per TS 29.510
		{
			name:    "Priority above max (70000)",
			field:   "Priority",
			value:   int32(70000),
			wantErr: true,
		},
		{
			name:    "Priority at max (65535)",
			field:   "Priority",
			value:   int32(65535),
			wantErr: false,
		},
		{
			name:    "Priority at min (0)",
			field:   "Priority",
			value:   int32(0),
			wantErr: false,
		},
		// SNssais.Sst: must be in [0, 255] per TS 23.501
		{
			name:    "SNssais Sst above max (256)",
			field:   "SNssais",
			value:   &[]models.Snssai{{Sst: 256}},
			wantErr: true,
		},
		{
			name:    "SNssais Sst at max (255)",
			field:   "SNssais",
			value:   &[]models.Snssai{{Sst: 255}},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile := map[string]any{}
			for k, v := range baseProfile {
				profile[k] = v
			}
			profile[tc.field] = tc.value

			_, err := Decode([]map[string]any{profile}, time.RFC3339)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %s = %v, got nil", tc.field, tc.value)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %s = %v: %v", tc.field, tc.value, err)
			}
		})
	}
}

// TestDecodeJSONStringHook verifies that the JSON-string decode hook correctly
// unmarshals JSON-encoded strings into their destination slice and map types.
// This covers the DB-retrieval use case where values are stored as JSON strings
// rather than their native Go types.
func TestDecodeJSONStringHook(t *testing.T) {
	cases := []struct {
		name        string
		field       string
		jsonValue   string
		checkResult func(t *testing.T, p models.NFProfileDiscovery)
	}{
		{
			// Direct slice target: NsiList []string
			name:      "NsiList from JSON string",
			field:     "NsiList",
			jsonValue: `["nsi0","nsi1"]`,
			checkResult: func(t *testing.T, p models.NFProfileDiscovery) {
				got, ok := p.GetNsiListOk()
				if !ok || len(got) != 2 || got[0] != "nsi0" || got[1] != "nsi1" {
					t.Fatalf("unexpected NsiList: %v", got)
				}
			},
		},
		{
			// Pointer-to-slice target: AllowedNssais *[]models.Snssai
			name:      "AllowedNssais from JSON string",
			field:     "AllowedNssais",
			jsonValue: `[{"sst":1,"sd":"010203"}]`,
			checkResult: func(t *testing.T, p models.NFProfileDiscovery) {
				got, ok := p.GetAllowedNssaisOk()
				if !ok || len(got) != 1 || got[0].GetSst() != 1 {
					t.Fatalf("unexpected AllowedNssais: %v", got)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile := map[string]any{
				"NfInstanceId": "json-hook-test",
				"NfType":       models.NFTYPE_NRF,
				"NfStatus":     models.NFSTATUS_REGISTERED,
				tc.field:       tc.jsonValue,
			}
			result, err := Decode([]map[string]any{profile}, time.RFC3339)
			if err != nil {
				t.Fatalf("Decode returned error: %v", err)
			}
			if len(result) != 1 {
				t.Fatalf("expected 1 profile, got %d", len(result))
			}
			tc.checkResult(t, result[0])
		})
	}
}

func TestConvertNFProfileDiscoveryToNFProfile(t *testing.T) {
	recoveryTime := time.Now().UTC().Truncate(time.Second)
	discovery := models.NFProfileDiscovery{
		NfInstanceId:  "instance-1",
		NfType:        models.NFTYPE_UDM,
		NfStatus:      models.NFSTATUS_REGISTERED,
		Priority:      openapi.PtrInt32(7),
		Ipv4Addresses: []string{"10.0.0.1"},
		PlmnList: []models.PlmnId{{
			Mcc: "001",
			Mnc: "01",
		}},
		RecoveryTime: &recoveryTime,
	}

	profile := ConvertNFProfileDiscoveryToNFProfile(discovery)

	if profile.GetNfInstanceId() != discovery.GetNfInstanceId() {
		t.Fatalf("expected NF instance ID %q, got %q", discovery.GetNfInstanceId(), profile.GetNfInstanceId())
	}
	if profile.GetNfType() != discovery.GetNfType() {
		t.Fatalf("expected NF type %q, got %q", discovery.GetNfType(), profile.GetNfType())
	}
	if profile.GetPriority() != discovery.GetPriority() {
		t.Fatalf("expected priority %d, got %d", discovery.GetPriority(), profile.GetPriority())
	}
	if got := profile.GetIpv4Addresses(); len(got) != 1 || got[0] != "10.0.0.1" {
		t.Fatalf("unexpected ipv4 addresses: %+v", got)
	}
	plmnList, ok := profile.GetPlmnListOk()
	if !ok || len(plmnList) != 1 || plmnList[0].Mcc != "001" || plmnList[0].Mnc != "01" {
		t.Fatalf("unexpected PLMN list: %+v", plmnList)
	}
	if profile.GetRecoveryTime() != recoveryTime {
		t.Fatalf("expected recovery time %v, got %v", recoveryTime, profile.GetRecoveryTime())
	}
}
