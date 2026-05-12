// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

var NrfNfProfile models.NFProfile

func InitNrfContext() {
	config := factory.NrfConfig
	logger.InitLog.Infof("nrfconfig Info: Version[%s] Description[%s]", config.Info.Version, config.Info.Description)
	configuration := config.Configuration

	NrfNfProfile.NfInstanceId = uuid.New().String()
	NrfNfProfile.NfType = models.NFTYPE_NRF
	NrfNfProfile.NfStatus = models.NFSTATUS_REGISTERED

	serviceNameList := configuration.ServiceNameList
	NFServices := InitNFService(serviceNameList, config.Info.Version)
	NrfNfProfile.NfServices = NFServices
}

func InitNFService(srvNameList []string, version string) []models.NFService {
	tmpVersion := strings.Split(version, ".")
	versionUri := "v" + tmpVersion[0]
	NFServices := make([]models.NFService, len(srvNameList))
	for index, nameString := range srvNameList {
		name := models.ServiceName(nameString)
		NFServices[index] = models.NFService{
			ServiceInstanceId: strconv.Itoa(index),
			ServiceName:       name,
			Versions: []models.NFServiceVersion{
				{
					ApiFullVersion:  version,
					ApiVersionInUri: versionUri,
				},
			},
			Scheme:          models.UriScheme(factory.NrfConfig.GetSbiScheme()),
			NfServiceStatus: models.NFSERVICESTATUS_REGISTERED,
			ApiPrefix:       openapi.PtrString(factory.NrfConfig.GetSbiUri()),
			IpEndPoints: []models.IpEndPoint{
				{
					Ipv4Address: openapi.PtrString(factory.NrfConfig.GetSbiRegisterIP()),
					Transport:   models.TRANSPORTPROTOCOL_TCP.Ptr(),
					Port:        openapi.PtrInt32(int32(factory.NrfConfig.GetSbiPort())),
				},
			},
		}
	}
	return NFServices
}
