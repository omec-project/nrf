// SPDX-FileCopyrightText: 2025 Canonical Ltd
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
	"path/filepath"
	"sync"
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
	openapiLogger "github.com/omec-project/openapi/logger"
	"github.com/omec-project/util/http2_util"
	utilLogger "github.com/omec-project/util/logger"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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

var initLog *zap.SugaredLogger

func init() {
	initLog = logger.InitLog
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

	if factory.NrfConfig.Logger.Util != nil {
		if factory.NrfConfig.Logger.Util.DebugLevel != "" {
			if level, err := zapcore.ParseLevel(factory.NrfConfig.Logger.Util.DebugLevel); err != nil {
				utilLogger.UtilLog.Warnf("Util Log level [%s] is invalid, set to [info] level",
					factory.NrfConfig.Logger.Util.DebugLevel)
				utilLogger.SetLogLevel(zap.InfoLevel)
			} else {
				utilLogger.SetLogLevel(level)
			}
		} else {
			utilLogger.UtilLog.Warnln("Util Log level not set. Default set to [info] level")
			utilLogger.SetLogLevel(zap.InfoLevel)
		}
	}
}

func (nrf *NRF) FilterCli(c *cli.Command) (args []string) {
	for _, flag := range nrf.GetCliCmd() {
		name := flag.Names()[0]
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
	initLog.Infof("binding addr: [%s]", bindAddr)
	sslLog := filepath.Dir(factory.NrfConfig.CfgLocation) + "/sslkey.log"
	server, err := http2_util.NewServer(bindAddr, sslLog, router)

	if server == nil {
		initLog.Errorf("initialize HTTP server failed: %+v", err)
		return
	}

	if err != nil {
		initLog.Warnf("initialize HTTP server: +%v", err)
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
		initLog.Fatalf("HTTP server setup failed: %+v", err)
	}
}

func (nrf *NRF) Exec(c *cli.Command) error {
	initLog.Debugln("args:", c.String("cfg"))
	args := nrf.FilterCli(c)
	initLog.Debugln("filter:", args)
	command := exec.Command("nrf", args...)

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
			initLog.Infof("NRF start error: %v", err)
		}
		initLog.Infoln("NRF end")
		wg.Done()
	}()

	wg.Wait()

	return err
}

func (nrf *NRF) Terminate() {
	logger.InitLog.Infoln("terminating NRF")
	logger.InitLog.Infoln("NRF terminated")
}
