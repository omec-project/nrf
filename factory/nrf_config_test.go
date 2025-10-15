// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2024 Canonical Ltd.
/*
 *  Tests for NRF Configuration Factory
 */

package factory

import (
	"testing"
)

func TestWebuiUrl(t *testing.T) {
	tests := []struct {
		name       string
		configFile string
		want       string
	}{
		{
			name:       "default webui URL",
			configFile: "../nrfTest/nrfcfg.yaml",
			want:       "http://webui:5001",
		},
		{
			name:       "custom webui URL",
			configFile: "../nrfTest/nrfcfg_with_custom_webui_url.yaml",
			want:       "https://myspecialwebui:5002",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origNrfConfig := NrfConfig
			defer func() { NrfConfig = origNrfConfig }()

			if err := InitConfigFactory(tt.configFile); err != nil {
				t.Fatalf("error in InitConfigFactory: %v", err)
			}

			got := NrfConfig.Configuration.WebuiUri
			if got != tt.want {
				t.Errorf("The webui URL is not correct. got = %q, want = %q", got, tt.want)
			}
		})
	}
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
