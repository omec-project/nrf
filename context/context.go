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
	"github.com/omec-project/openapi/models"
)

type NRFContext struct {
	NrfNfProfile     models.NfProfile
	Nrf_NfInstanceID string
	PlmnList         []models.PlmnId
}

var nrfContext NRFContext

func Init() {
	InitNrfContext(&nrfContext)
}

func InitNrfContext(context *NRFContext) {
	config := factory.NrfConfig
	logger.InitLog.Infof("nrfconfig Info: Version[%s] Description[%s]", config.Info.Version, config.Info.Description)
	configuration := config.Configuration

	context.NrfNfProfile.NfInstanceId = uuid.New().String()
	context.NrfNfProfile.NfType = models.NfType_NRF
	context.NrfNfProfile.NfStatus = models.NfStatus_REGISTERED

	serviceNameList := configuration.ServiceNameList
	context.PlmnList = []models.PlmnId{}
	NFServices := InitNFService(serviceNameList, config.Info.Version)
	context.NrfNfProfile.NfServices = &NFServices
	logger.ContextLog.Infoln("nrf context:", context)
}

func InitNFService(srvNameList []string, version string) []models.NfService {
	tmpVersion := strings.Split(version, ".")
	versionUri := "v" + tmpVersion[0]
	NFServices := make([]models.NfService, len(srvNameList))
	for index, nameString := range srvNameList {
		name := models.ServiceName(nameString)
		NFServices[index] = models.NfService{
			ServiceInstanceId: strconv.Itoa(index),
			ServiceName:       name,
			Versions: &[]models.NfServiceVersion{
				{
					ApiFullVersion:  version,
					ApiVersionInUri: versionUri,
				},
			},
			Scheme:          models.UriScheme(factory.NrfConfig.GetSbiScheme()),
			NfServiceStatus: models.NfServiceStatus_REGISTERED,
			ApiPrefix:       factory.NrfConfig.GetSbiUri(),
			IpEndPoints: &[]models.IpEndPoint{
				{
					Ipv4Address: factory.NrfConfig.GetSbiRegisterIP(),
					Transport:   models.TransportProtocol_TCP,
					Port:        int32(factory.NrfConfig.GetSbiPort()),
				},
			},
		}
	}
	return NFServices
}

func GetSelf() *NRFContext {
	return &nrfContext
}
