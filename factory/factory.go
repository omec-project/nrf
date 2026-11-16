// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/util/nfconfig"
)

var NrfConfig Config

// InitConfigFactory gets the NrfConfig and sets the REST API endpoint used to
// fetch the configuration from.
func InitConfigFactory(f string) error {
	NrfConfig = Config{}

	if err := nfconfig.LoadYAML(f, &NrfConfig); err != nil {
		return err
	}
	webuiUri, err := nfconfig.ResolveWebuiUri(NrfConfig.Configuration.WebuiUri, logger.CfgLog)
	if err != nil {
		return err
	}
	NrfConfig.Configuration.WebuiUri = webuiUri
	return nil
}

func CheckConfigVersion() error {
	return nfconfig.CheckVersion(NrfConfig.GetVersion(), NRF_EXPECTED_CONFIG_VERSION, logger.CfgLog)
}
