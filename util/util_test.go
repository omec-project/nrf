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

	"github.com/omec-project/openapi"
	"github.com/omec-project/openapi/models"
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
		t.Log(err)
	}

	t.Logf("%+v", target)
}
