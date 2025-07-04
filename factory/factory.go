// SPDX-FileCopyrightText: 2025 Canonical Ltd
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
	"net/url"
	"os"

	"github.com/omec-project/nrf/logger"
	"gopkg.in/yaml.v2"
)

var NrfConfig Config

// InitConfigFactory gets the NrfConfig and sets the REST API endpoint used to
// fetch the configuration from.
func InitConfigFactory(f string) error {
	content, err := os.ReadFile(f)
	if err != nil {
		return err
	}
	NrfConfig = Config{}

	if err = yaml.Unmarshal(content, &NrfConfig); err != nil {
		return err
	}
	if NrfConfig.Configuration.WebuiUri == "" {
		NrfConfig.Configuration.WebuiUri = "http://webui:5001"
		logger.CfgLog.Infof("webuiUri not set in configuration file. Using %v", NrfConfig.Configuration.WebuiUri)
		return nil
	}
	err = validateWebuiUri(NrfConfig.Configuration.WebuiUri)
	return err
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

func validateWebuiUri(uri string) error {
	parsedUrl, err := url.ParseRequestURI(uri)
	if err != nil {
		return err
	}
	if parsedUrl.Scheme != "http" && parsedUrl.Scheme != "https" {
		return fmt.Errorf("unsupported scheme for webuiUri: %s", parsedUrl.Scheme)
	}
	if parsedUrl.Hostname() == "" {
		return fmt.Errorf("missing host in webuiUri")
	}
	return nil
}
