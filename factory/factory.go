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

	grpcClient "github.com/omec-project/config5g/proto/client"
	protos "github.com/omec-project/config5g/proto/sdcoreConfig"
	"github.com/omec-project/nrf/logger"
	"gopkg.in/yaml.v2"
)

var ManagedByConfigPod bool

var NrfConfig Config

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
		logger.InitLog.Infof("DefaultPlmnId Mnc %v, Mcc %v", NrfConfig.Configuration.DefaultPlmnId.Mnc, NrfConfig.Configuration.DefaultPlmnId.Mcc)
		if os.Getenv("MANAGED_BY_CONFIG_POD") == "true" {
			logger.InitLog.Infoln("MANAGED_BY_CONFIG_POD is true")
			client, err := grpcClient.ConnectToConfigServer(NrfConfig.Configuration.WebuiUri)
			if err != nil {
				go updateConfig(client)
			}
			return err
		}
	}
	return nil
}

// updateConfig connects the config pod GRPC server and subscribes the config changes
// then updates NRF configuration
func updateConfig(client grpcClient.ConfClient) {
	var stream protos.ConfigService_NetworkSliceSubscribeClient
	var err error
	var configChannel chan *protos.NetworkSliceResponse
	for {
		if client != nil {
			stream, err = client.CheckGrpcConnectivity()
			if err != nil {
				logger.InitLog.Errorf("%v", err)
				if stream != nil {
					time.Sleep(time.Second * 30)
					continue
				} else {
					err = client.GetConfigClientConn().Close()
					if err != nil {
						logger.InitLog.Debugf("failing ConfigClient is not closed properly: %+v", err)
					}
					client = nil
					continue
				}
			}
			if configChannel == nil {
				configChannel = client.PublishOnConfigChange(true, stream)
				ManagedByConfigPod = true
				go NrfConfig.updateConfig(configChannel)
			}

		} else {
			client, err = grpcClient.ConnectToConfigServer(NrfConfig.Configuration.WebuiUri)
			if err != nil {
				logger.InitLog.Errorf("%+v", err)
			}
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
