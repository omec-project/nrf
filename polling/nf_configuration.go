// SPDX-FileCopyrightText: 2025 Canonical Ltd

// SPDX-License-Identifier: Apache-2.0
//

package polling

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	nrfContext "github.com/omec-project/nrf/context"
	"github.com/omec-project/nrf/factory"
	"github.com/omec-project/nrf/logger"
	"github.com/omec-project/openapi/models"
)

const (
	INITIAL_POLLING_INTERVAL = 5 * time.Second
	POLLING_MAX_BACKOFF      = 40 * time.Second
	POLLING_BACKOFF_FACTOR   = 2
	POLLING_PATH             = "/nfconfig/plmn"
)

// PollNetworkConfig makes a HTTP GET request to the webconsole and updates the network configuration
func PollNetworkConfig() {
	interval := INITIAL_POLLING_INTERVAL
	nrfContext := nrfContext.GetSelf()

	for {
		time.Sleep(interval)
		newPlmnConfig, err := fetchPlmnConfig()
		if err != nil {
			interval = minDuration(interval*time.Duration(POLLING_BACKOFF_FACTOR), POLLING_MAX_BACKOFF)
			logger.PollConfigLog.Errorf("error polling network configuration. Will retry in %v: ", interval, err)
			continue
		}
		logger.PollConfigLog.Infoln("configuration polled successfully")
		interval = INITIAL_POLLING_INTERVAL
		handlePolledPlmnConfig(nrfContext, newPlmnConfig)
	}
}

func fetchPlmnConfig() ([]models.PlmnId, error) {
	pollingEndpoint := factory.NrfConfig.Configuration.WebuiUri + POLLING_PATH
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollingEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET %v failed: %w", pollingEndpoint, err)
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return nil, fmt.Errorf("unexpected Content-Type: got %s, want application/json", contentType)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		var config []models.PlmnId
		if err := json.Unmarshal(body, &config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON response: %w", err)
		}
		return config, nil

	case http.StatusBadRequest, http.StatusInternalServerError:
		return nil, fmt.Errorf("server returned %d error code", resp.StatusCode)
	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

func handlePolledPlmnConfig(nrfContext *nrfContext.NRFContext, newPlmnConfig []models.PlmnId) {
	if reflect.DeepEqual(nrfContext.PlmnList, newPlmnConfig) {
		logger.PollConfigLog.Debugln("PLMN config did not change")
		return
	}
	nrfContext.PlmnList = newPlmnConfig
	logger.PollConfigLog.Infof("PLMN config changed. New PLMN ID list: %v", nrfContext.PlmnList)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
