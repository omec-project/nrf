// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-License-Identifier: Apache-2.0

package polling

import (
	"context"
	"net/http"
	"time"

	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/nfconfigpoll"
)

const (
	nfconfigPlmnEndpoint = "/nfconfig/plmn"
	contentTypeJSON      = "application/json"
)

var FetchPlmnConfig = func() ([]models.PlmnId, error) {
	plmnConfigEndpoint := factory.NrfConfig.Configuration.WebuiUri + nfconfigPlmnEndpoint
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return nfconfigpoll.FetchJSON[[]models.PlmnId](ctx, &http.Client{}, plmnConfigEndpoint)
}
