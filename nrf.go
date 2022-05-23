// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

/*
 * API version: 1.0.0
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/omec-project/nrf/logger"
	nrf_service "github.com/omec-project/nrf/service"
)

var NRF = &nrf_service.NRF{}

var appLog *logrus.Entry

func init() {
	appLog = logger.AppLog
}

var (
	VERSION     string
	BUILD_TIME  string
	COMMIT_HASH string
	COMMIT_TIME string
)

func GetVersion() string {
	if VERSION != "" {
		return fmt.Sprintf(
			"\n\tfree5GC version: %s"+
				"\n\tbuild time:      %s"+
				"\n\tcommit hash:     %s"+
				"\n\tcommit time:     %s"+
				"\n\tgo version:      %s %s/%s",
			VERSION,
			BUILD_TIME,
			COMMIT_HASH,
			COMMIT_TIME,
			runtime.Version(),
			runtime.GOOS,
			runtime.GOARCH,
		)
	} else {
		return fmt.Sprintf(
			"\n\tNot specify ldflags (which link version) during go build\n\tgo version: %s %s/%s",
			runtime.Version(),
			runtime.GOOS,
			runtime.GOARCH,
		)
	}
}
func main() {
	app := cli.NewApp()
	app.Name = "nrf"
	fmt.Print(app.Name, "\n")
	appLog.Infoln("NRF version: ", GetVersion())
	app.Usage = "-free5gccfg common configuration file -nrfcfg nrf configuration file"
	app.Action = action
	app.Flags = NRF.GetCliCmd()

	if err := app.Run(os.Args); err != nil {
		appLog.Errorf("NRF Run Error: %v", err)
	}
}

func action(c *cli.Context) error {
	if err := NRF.Initialize(c); err != nil {
		logger.CfgLog.Errorf("%+v", err)
		return fmt.Errorf("Failed to initialize !!")
	}

	NRF.Start()

	return nil
}
