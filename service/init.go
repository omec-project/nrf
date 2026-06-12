// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
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
	sslLog := filepath.Dir(factory.NrfConfig.CfgLocation) + "/sslkey.log"
	server, err := http2_util.NewServer(bindAddr, sslLog, router)

	if server == nil {
		logger.InitLog.Errorf("initialize HTTP server failed: %+v", err)
		return
	}

	if err != nil {
		logger.InitLog.Warnf("initialize HTTP server: %+v", err)
	}

	serverScheme := factory.NrfConfig.GetSbiScheme()
	switch serverScheme {
	case "http":
		err = server.ListenAndServe()
	case "https":
		err = server.ListenAndServeTLS(config.Sbi.TLS.PEM, config.Sbi.TLS.Key)
	default:
		logger.InitLog.Fatalf("HTTP server setup failed: invalid server scheme %+v", serverScheme)
		return
	}

	if err != nil {
		logger.InitLog.Fatalf("HTTP server setup failed: %+v", err)
	}
}

func (nrf *NRF) Terminate() {
	logger.InitLog.Infoln("terminating NRF")
	logger.InitLog.Infoln("NRF terminated")
}
