// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/urfave/cli"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/omec-project/nrf/accesstoken"
	nrfContext "github.com/omec-project/nrf/context"
	"github.com/omec-project/nrf/dbadapter"
	"github.com/omec-project/nrf/discovery"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/nrf/management"
	"github.com/omec-project/nrf/metrics"
	"github.com/omec-project/nrf/util"
	openapiLogger "github.com/omec-project/openapi/logger"
	"github.com/omec-project/util/http2_util"
	utilLogger "github.com/omec-project/util/logger"
	"github.com/omec-project/util/path_util"
)

type NRF struct{}

type (
	// Config information.
	Config struct {
		nrfcfg string
	}
)

var config Config

var nrfCLi = []cli.Flag{
	cli.StringFlag{
		Name:  "free5gccfg",
		Usage: "common config file",
	},
	cli.StringFlag{
		Name:  "nrfcfg",
		Usage: "config file",
	},
}

var initLog *zap.SugaredLogger

func init() {
	initLog = logger.InitLog
}

func (*NRF) GetCliCmd() (flags []cli.Flag) {
	return nrfCLi
}

func (nrf *NRF) Initialize(c *cli.Context) error {
	config = Config{
		nrfcfg: c.String("nrfcfg"),
	}

	if config.nrfcfg != "" {
		if err := factory.InitConfigFactory(config.nrfcfg); err != nil {
			return err
		}
	} else {
		DefaultNrfConfigPath := path_util.Free5gcPath("free5gc/config/nrfcfg.yaml")
		if err := factory.InitConfigFactory(DefaultNrfConfigPath); err != nil {
			return err
		}
	}

	nrf.setLogLevel()

	if err := factory.CheckConfigVersion(); err != nil {
		return err
	}

	return nil
}

func (nrf *NRF) setLogLevel() {
	if factory.NrfConfig.Logger == nil {
		initLog.Warnln("NRF config without log level setting!!!")
		return
	}

	if factory.NrfConfig.Logger.NRF != nil {
		if factory.NrfConfig.Logger.NRF.DebugLevel != "" {
			level, err := zapcore.ParseLevel(factory.NrfConfig.Logger.NRF.DebugLevel)
			if err != nil {
				initLog.Warnf("NRF Log level [%s] is invalid, set to [info] level",
					factory.NrfConfig.Logger.NRF.DebugLevel)
				logger.SetLogLevel(zap.InfoLevel)
			} else {
				initLog.Infof("NRF Log level is set to [%s] level", level)
				logger.SetLogLevel(level)
			}
		} else {
			initLog.Infoln("NRF Log level not set. Default set to [info] level")
			logger.SetLogLevel(zap.InfoLevel)
		}
	}

	if factory.NrfConfig.Logger.OpenApi != nil {
		if factory.NrfConfig.Logger.OpenApi.DebugLevel != "" {
			if _, err := zapcore.ParseLevel(factory.NrfConfig.Logger.OpenApi.DebugLevel); err != nil {
				openapiLogger.OpenapiLog.Warnf("OpenAPI Log level [%s] is invalid, set to [info] level",
					factory.NrfConfig.Logger.OpenApi.DebugLevel)
				logger.SetLogLevel(zap.InfoLevel)
			}
		} else {
			openapiLogger.OpenapiLog.Warnln("OpenAPI Log level not set. Default set to [info] level")
			logger.SetLogLevel(zap.InfoLevel)
		}
	}

	if factory.NrfConfig.Logger.MongoDBLibrary != nil {
		if factory.NrfConfig.Logger.MongoDBLibrary.DebugLevel != "" {
			if level, err := zapcore.ParseLevel(factory.NrfConfig.Logger.MongoDBLibrary.DebugLevel); err != nil {
				utilLogger.AppLog.Warnf("MongoDBLibrary Log level [%s] is invalid, set to [info] level",
					factory.NrfConfig.Logger.MongoDBLibrary.DebugLevel)
				utilLogger.SetLogLevel(zap.InfoLevel)
			} else {
				utilLogger.SetLogLevel(level)
			}
		} else {
			utilLogger.AppLog.Warnln("MongoDBLibrary Log level not set. Default set to [info] level")
			utilLogger.SetLogLevel(zap.InfoLevel)
		}
	}
}

func (nrf *NRF) FilterCli(c *cli.Context) (args []string) {
	for _, flag := range nrf.GetCliCmd() {
		name := flag.GetName()
		value := fmt.Sprint(c.Generic(name))
		if value == "" {
			continue
		}

		args = append(args, "--"+name, value)
	}
	return args
}

func (nrf *NRF) Start() {
	initLog.Infoln("server started")
	dbadapter.ConnectToDBClient(factory.NrfConfig.Configuration.MongoDBName, factory.NrfConfig.Configuration.MongoDBUrl,
		factory.NrfConfig.Configuration.MongoDBStreamEnable, factory.NrfConfig.Configuration.NfProfileExpiryEnable)

	router := utilLogger.NewGinWithZap(logger.GinLog)

	accesstoken.AddService(router)
	discovery.AddService(router)
	management.AddService(router)

	go metrics.InitMetrics()

	nrfContext.InitNrfContext()

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChannel
		// Waiting for other NFs to deregister
		time.Sleep(2 * time.Second)
		nrf.Terminate()
		os.Exit(0)
	}()

	if os.Getenv("MANAGED_BY_CONFIG_POD") == "true" {
		initLog.Infoln("MANAGED_BY_CONFIG_POD is true")
	} else {
		initLog.Infoln("Use helm chart config")
	}
	bindAddr := factory.NrfConfig.GetSbiBindingAddr()
	initLog.Infof("Binding addr: [%s]", bindAddr)
	server, err := http2_util.NewServer(bindAddr, util.NrfLogPath, router)

	if server == nil {
		initLog.Errorf("Initialize HTTP server failed: %+v", err)
		return
	}

	if err != nil {
		initLog.Warnf("Initialize HTTP server: +%v", err)
	}

	serverScheme := factory.NrfConfig.GetSbiScheme()
	if serverScheme == "http" {
		err = server.ListenAndServe()
	} else if serverScheme == "https" {
		err = server.ListenAndServeTLS(util.NrfPemPath, util.NrfKeyPath)
	}

	if err != nil {
		initLog.Fatalf("HTTP server setup failed: %+v", err)
	}
}

func (nrf *NRF) Exec(c *cli.Context) error {
	initLog.Debugln("args:", c.String("nrfcfg"))
	args := nrf.FilterCli(c)
	initLog.Debugln("filter:", args)
	command := exec.Command("./nrf", args...)

	if err := nrf.Initialize(c); err != nil {
		return err
	}

	stdout, err := command.StdoutPipe()
	if err != nil {
		initLog.Fatalln(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() {
		in := bufio.NewScanner(stdout)
		for in.Scan() {
			initLog.Infoln(in.Text())
		}
		wg.Done()
	}()

	stderr, err := command.StderrPipe()
	if err != nil {
		initLog.Fatalln(err)
	}
	go func() {
		in := bufio.NewScanner(stderr)
		initLog.Infoln("NRF log start")
		for in.Scan() {
			initLog.Infoln(in.Text())
		}
		wg.Done()
	}()

	go func() {
		initLog.Infoln("NRF start")
		if err = command.Start(); err != nil {
			initLog.Infof("NRF Start error: %v", err)
		}
		initLog.Infoln("NRF end")
		wg.Done()
	}()

	wg.Wait()

	return err
}

func (nrf *NRF) Terminate() {
	logger.InitLog.Infoln("terminating NRF...")
	logger.InitLog.Infoln("NRF terminated")
}
