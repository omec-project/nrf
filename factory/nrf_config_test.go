// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2024 Canonical Ltd.
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/omec-project/util/nfconfig"
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
			want:       nfconfig.DefaultWebuiUri,
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

// An HTTP configuration has no sbi.tls block, and nothing may read the certificate paths of one: the service
// takes them before it starts the listener, whatever the scheme is.
func TestGetSbiTLSPaths(t *testing.T) {
	cases := map[string]struct {
		cfg      Config
		pem, key string
	}{
		"no configuration": {Config{}, "", ""},
		"no sbi":           {Config{Configuration: &Configuration{}}, "", ""},
		"http without tls": {Config{Configuration: &Configuration{Sbi: &Sbi{Scheme: "http"}}}, "", ""},
		"https with tls": {
			Config{Configuration: &Configuration{Sbi: &Sbi{Scheme: "https", TLS: &TLS{PEM: "/tls/nrf.pem", Key: "/tls/nrf.key"}}}},
			"/tls/nrf.pem", "/tls/nrf.key",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			pem, key := tc.cfg.GetSbiTLSPaths()
			if pem != tc.pem || key != tc.key {
				t.Errorf("got %q, %q; want %q, %q", pem, key, tc.pem, tc.key)
			}
		})
	}
}

// The same, from the YAML an operator writes for HTTP: scheme http and no tls block.
func TestGetSbiTLSPathsFromAnHttpConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nrfcfg.yaml")
	yaml := "info:\n  version: 1.0.0\nconfiguration:\n  sbi:\n    scheme: http\n    registerIPv4: 127.0.0.1\n    bindingIPv4: 127.0.0.1\n    port: 8000\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InitConfigFactory(path); err != nil {
		t.Fatal(err)
	}
	if pem, key := NrfConfig.GetSbiTLSPaths(); pem != "" || key != "" || NrfConfig.GetSbiScheme() != "http" {
		t.Errorf("scheme %q, paths %q %q", NrfConfig.GetSbiScheme(), pem, key)
	}
}
