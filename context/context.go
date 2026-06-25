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
	"github.com/omec-project/openapi/v2/models"
)

var NrfNfProfile models.NFProfile

func InitNrfContext() {
	config := factory.NrfConfig
	logger.InitLog.Infof("nrfconfig Info: Version[%s] Description[%s]", config.Info.Version, config.Info.Description)
	configuration := config.Configuration

	NrfNfProfile.SetNfInstanceId(uuid.New().String())
	NrfNfProfile.SetNfType(models.NFTYPE_NRF)
	NrfNfProfile.SetNfStatus(models.NFSTATUS_REGISTERED)

	NFServices := InitNFService(configuration.ServiceNameList, config.Info.Version)
	NrfNfProfile.SetNfServices(NFServices)
}

func InitNFService(srvNameList []string, version string) []models.NFService {
	tmpVersion := strings.Split(version, ".")
	nfServices := make([]models.NFService, len(srvNameList))
	ipEndPoint := models.NewIpEndPoint()
	ipEndPoint.SetIpv4Address(factory.NrfConfig.GetSbiRegisterIP())
	ipEndPoint.SetTransport(models.TRANSPORTPROTOCOL_TCP)
	ipEndPoint.SetPort(int32(factory.NrfConfig.GetSbiPort()))
	scheme := models.UriScheme(factory.NrfConfig.GetSbiScheme())
	apiPrefix := factory.NrfConfig.GetSbiUri()
	nFServiceVersion := models.NewNFServiceVersion("v"+tmpVersion[0], version)
	for index, nameString := range srvNameList {
		serviceName := models.ServiceName(nameString)
		nfService := models.NewNFService(strconv.Itoa(index), serviceName, []models.NFServiceVersion{*nFServiceVersion}, scheme, models.NFSERVICESTATUS_REGISTERED)
		nfService.SetApiPrefix(apiPrefix)
		nfService.SetIpEndPoints([]models.IpEndPoint{*ipEndPoint})
		nfServices[index] = *nfService
	}
	return nfServices
}
