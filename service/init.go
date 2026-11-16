// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/omec-project/nrf/accesstoken"
	"github.com/omec-project/nrf/context"
	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/discovery"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/nrf/management"
	"github.com/omec-project/nrf/metrics"
	"github.com/omec-project/nrf/producer"
	nrfUtil "github.com/omec-project/nrf/util"
	openapiLogger "github.com/omec-project/openapi/v2/logger"
	"github.com/omec-project/util/http2_util"
	utilLogger "github.com/omec-project/util/logger"
	"github.com/urfave/cli/v3"
)

type NRF struct{}

type (
	// Config information.
	Config struct {
		cfg string
	}
)

var config Config

var nrfCLi = []cli.Flag{
	&cli.StringFlag{
		Name:     "cfg",
		Usage:    "nrf config file",
		Required: true,
	},
}

func (*NRF) GetCliCmd() (flags []cli.Flag) {
	return nrfCLi
}

func (nrf *NRF) Initialize(c *cli.Command) error {
	config = Config{
		cfg: c.String("cfg"),
	}

	absPath, err := filepath.Abs(config.cfg)
	if err != nil {
		logger.CfgLog.Errorln(err)
		return err
	}

	if err := factory.InitConfigFactory(absPath); err != nil {
		return err
	}

	nrf.setLogLevel()

	if err := factory.CheckConfigVersion(); err != nil {
		return err
	}

	factory.NrfConfig.CfgLocation = absPath

	context.InitNrfContext()

	// A key set explicitly via accessTokenKey is an operator decision, so a bad
	// one aborts startup. The sbi.tls.key fallback may not exist (e.g. plain
	// HTTP deployments): discovery and management must still start, and token
	// issuance stays fail-closed (HTTP 500) until a key is provisioned.
	if err := producer.LoadAccessTokenSigner(factory.NrfConfig.GetAccessTokenKeyPath()); err != nil {
		if factory.NrfConfig.Configuration != nil && factory.NrfConfig.Configuration.AccessTokenKey != "" {
			return err
		}
		logger.InitLog.Warnf("access token signing disabled, token requests will fail: %v "+
			"(set configuration.accessTokenKey to enable)", err)
	}

	return nil
}

func (nrf *NRF) setLogLevel() {
	cfgLogger := factory.NrfConfig.Logger
	if cfgLogger == nil {
		logger.InitLog.Warnln("NRF config without log level setting!!!")
		return
	}

	utilLogger.ApplyLogSetting("NRF", cfgLogger.NRF, logger.InitLog, logger.SetLogLevel)
	utilLogger.ApplyLogSetting("OpenApi", cfgLogger.OpenApi, openapiLogger.OpenapiLog, openapiLogger.SetLogLevel)
	utilLogger.ApplyLogSetting("Util", cfgLogger.Util, utilLogger.UtilLog, utilLogger.SetLogLevel)
}

func (nrf *NRF) Start() {
	logger.InitLog.Infoln("server started")
	config := factory.NrfConfig.Configuration
	dbadapter.ConnectToDBClient(config.MongoDBName, config.MongoDBUrl, config.MongoDBStreamEnable, config.NfProfileExpiryEnable)

	router := utilLogger.NewGinWithZap(logger.GinLog)

	// An SBI client cannot parse gin's plain-text "404 page not found", which is
	// what an unserved path or a method the API does not define for a resource
	// would otherwise return.
	nrfUtil.RegisterProblemHandlers(router, logger.GinLog)

	accesstoken.AddService(router)
	discovery.AddService(router)
	management.AddService(router)

	go metrics.InitMetrics()

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		// Waiting for other NFs to deregister
		time.Sleep(2 * time.Second)
		nrf.Terminate()
		os.Exit(0)
	}()

	bindAddr := factory.NrfConfig.GetSbiBindingAddr()
	logger.InitLog.Infof("binding addr: [%s]", bindAddr)
	// TLS key logging is a debugging aid: off unless the operator sets SSLKEYLOGFILE (see http2_util.NewSBIServer).
	pem, key := factory.NrfConfig.GetSbiTLSPaths()
	if err := http2_util.ListenAndServeSBI(bindAddr, router, factory.NrfConfig.GetSbiScheme(), pem, key, logger.InitLog); err != nil {
		logger.InitLog.Fatalf("HTTP server setup failed: %+v", err)
	}
}

func (nrf *NRF) Terminate() {
	logger.InitLog.Infoln("terminating NRF")
	logger.InitLog.Infoln("NRF terminated")
}
