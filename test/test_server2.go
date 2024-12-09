// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/http2_util"
	utilLogger "github.com/omec-project/util/logger"
)

func main() {
	router := utilLogger.NewGinWithZap(logger.GinLog)

	router.POST("", func(c *gin.Context) {
		var ND models.NotificationData

		if err := c.ShouldBindJSON(&ND); err != nil {
			logger.UtilLog.Panicln(err.Error())
		}
		logger.UtilLog.Infoln(ND)
		c.JSON(http.StatusNoContent, gin.H{})
	})

	srv, err := http2_util.NewServer(":30678", "/opt/sslkey", router)
	if err != nil {
		logger.UtilLog.Panicln(err.Error())
	}

	err2 := srv.ListenAndServeTLS("/var/run/certs/tls.crt", "/var/run/certs/tls.key")
	if err2 != nil && err2 != http.ErrServerClosed {
		logger.UtilLog.Panicln(err2.Error())
	}
}
