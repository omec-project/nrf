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

	nfServices := initNFService(configuration.ServiceNameList, config.Info.Version)
	NrfNfProfile.SetNfServices(nfServices)

	// nfServiceList: TS 29.510 Rel-16 replacement for nfServices, kept in sync
	// so consumers that only check nfServiceList (e.g. discovery) still see
	// the NRF's own services.
	nfServiceList := make(map[string]models.NFService, len(nfServices))
	for _, nfService := range nfServices {
		nfServiceList[nfService.GetServiceInstanceId()] = nfService
	}
	NrfNfProfile.SetNfServiceList(nfServiceList)
}

func initNFService(srvNameList []string, version string) []models.NFService {
	tmpVersion := strings.Split(version, ".")
	nfServices := make([]models.NFService, len(srvNameList))
	ipEndPoint := models.NewIpEndPoint()
	ipEndPoint.SetIpv4Address(factory.NrfConfig.GetSbiRegisterIP())
	ipEndPoint.SetTransport(models.TRANSPORTPROTOCOL_TCP)
	ipEndPoint.SetPort(int32(factory.NrfConfig.GetSbiPort()))
	scheme := models.UriScheme(factory.NrfConfig.GetSbiScheme())
	apiPrefix := factory.NrfConfig.GetSbiUri()
	nfServiceVersion := models.NewNFServiceVersion("v"+tmpVersion[0], version)
	for index, nameString := range srvNameList {
		serviceName := models.ServiceName(nameString)
		nfService := models.NewNFService(strconv.Itoa(index), serviceName, []models.NFServiceVersion{*nfServiceVersion}, scheme, models.NFSERVICESTATUS_REGISTERED)
		nfService.SetApiPrefix(apiPrefix)
		nfService.SetIpEndPoints([]models.IpEndPoint{*ipEndPoint})
		nfServices[index] = *nfService
	}
	return nfServices
}
