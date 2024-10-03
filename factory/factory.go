// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

/*
 * NRF Configuration Factory
 */

package factory

import (
	"fmt"
	"os"
	"time"

	protos "github.com/omec-project/config5g/proto/sdcoreConfig"
	"go.uber.org/zap"
	"google.golang.org/grpc/connectivity"
	"gopkg.in/yaml.v2"

	"github.com/omec-project/nrf/logger"
)

var ManagedByConfigPod bool

var NrfConfig Config

var initLog *zap.SugaredLogger

func init() {
	initLog = logger.InitLog
}

// InitConfigFactory gets the NrfConfig and subscribes the config pod.
// This observes the GRPC client availability and connection status in a loop.
// When the GRPC server pod is restarted, GRPC connection status stuck in idle.
// If GRPC client does not exist, creates it. If client exists but GRPC connectivity is not ready,
// then it closes the existing client start a new client.
// TODO: Support configuration update from REST api
func InitConfigFactory(f string) error {
	if content, err := os.ReadFile(f); err != nil {
		return err
	} else {
		NrfConfig = Config{}

		if yamlErr := yaml.Unmarshal(content, &NrfConfig); yamlErr != nil {
			return yamlErr
		}
		if NrfConfig.Configuration.WebuiUri == "" {
			NrfConfig.Configuration.WebuiUri = "webui:9876"
		}
		initLog.Infof("DefaultPlmnId Mnc %v , Mcc %v \n", NrfConfig.Configuration.DefaultPlmnId.Mnc, NrfConfig.Configuration.DefaultPlmnId.Mcc)
		roc := os.Getenv("MANAGED_BY_CONFIG_POD")
		if roc == "true" {
			initLog.Infoln("MANAGED_BY_CONFIG_POD is true")
			var client ConfClient
			client = ConnectToConfigServer(NrfConfig.Configuration.WebuiUri)
			go UpdateConfig(client)
		}
	}
	return nil
}

// UpdateConfig connects the config pod GRPC server and subscribes the config changes
// then updates NRF configuration
func UpdateConfig(client ConfClient) {
	var stream protos.ConfigService_NetworkSliceSubscribeClient
	var configChannel chan *protos.NetworkSliceResponse
	for {
		if client != nil {
			stream = client.ConnectToGrpcServer()
			if stream == nil {
				time.Sleep(time.Second * 30)
				continue
			}
			time.Sleep(time.Second * 30)
			if client.GetConfigClientConn().GetState() != connectivity.Ready {
				err := client.GetConfigClientConn().Close()
				if err != nil {
					initLog.Debugf("failing ConfigClient is not closed properly: %+v", err)
				}
				client = nil
				continue
			}
			if configChannel == nil {
				configChannel = client.PublishOnConfigChange(true, stream)
				ManagedByConfigPod = true
				go NrfConfig.updateConfig(configChannel)
			}

		} else {
			client = ConnectToConfigServer(NrfConfig.Configuration.WebuiUri)
			continue
		}

	}

}

func CheckConfigVersion() error {
	currentVersion := NrfConfig.GetVersion()

	if currentVersion != NRF_EXPECTED_CONFIG_VERSION {
		return fmt.Errorf("config version is [%s], but expected is [%s]",
			currentVersion, NRF_EXPECTED_CONFIG_VERSION)
	}

	logger.CfgLog.Infof("config version [%s]", currentVersion)

	return nil
}
