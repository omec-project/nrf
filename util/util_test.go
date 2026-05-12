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
				Sst: 222,
				Sd:  openapi.PtrString("SNssais"),
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
				Sst: 333,
				Sd:  openapi.PtrString("AllowedNssais"),
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
