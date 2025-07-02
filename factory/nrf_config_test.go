// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2024 Canonical Ltd.
/*
 *  Tests for NRF Configuration Factory
 */

package factory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Webui URL is not set then default Webui URL value is returned
func TestGetDefaultWebuiUrl(t *testing.T) {
	if err := InitConfigFactory("../nrfTest/nrfcfg.yaml"); err != nil {
		t.Errorf("error in InitConfigFactory: %v", err)
	}
	got := NrfConfig.Configuration.WebuiUri
	want := "http://webui:5001"
	assert.Equal(t, got, want, "The webui URL is not correct.")
}

// Webui URL is set to a custom value then custom Webui URL is returned
func TestGetCustomWebuiUrl(t *testing.T) {
	if err := InitConfigFactory("../nrfTest/nrfcfg_with_custom_webui_url.yaml"); err != nil {
		t.Errorf("error in InitConfigFactory: %v", err)
	}
	got := NrfConfig.Configuration.WebuiUri
	want := "https://myspecialwebui:5002"
	assert.Equal(t, got, want, "The webui URL is not correct.")
}

func TestValidateWebuiUri(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		isValid bool
	}{
		{
			name:    "Valid HTTPS URI with port",
			uri:     "https://webui:9090",
			isValid: true,
		},
		{
			name:    "Valid HTTP URI with port",
			uri:     "http://webui:8080",
			isValid: true,
		},
		{
			name:    "Invalid scheme",
			uri:     "ftp://webui:21",
			isValid: false,
		},
		{
			name:    "URI is missing scheme",
			uri:     "webui:9090",
			isValid: false,
		},
		{
			name:    "URI is missing host",
			uri:     "https://",
			isValid: false,
		},
		{
			name:    "URI is empty string",
			uri:     "",
			isValid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWebuiUri(tc.uri)
			if err == nil && !tc.isValid {
				t.Errorf("expected URI: %s to be invalid", tc.uri)
			}
			if err != nil && tc.isValid {
				t.Errorf("expected URI: %s to be valid", tc.uri)
			}
		})
	}
}
