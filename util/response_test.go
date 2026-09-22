// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/omec-project/nrf/util"
)

type quietLogger struct{}

func (quietLogger) Warnf(string, ...any) {}

// gin's defaults for an unserved path and an undefined method are plain text,
// which an SBI client cannot parse. The API definitions type every error
// response as application/problem+json.
func TestRegisterProblemHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// Deliberately not setting HandleMethodNotAllowed here: registering the
	// handlers must be enough, or the production wiring in service/init.go
	// would never reach the NoMethod path.
	util.RegisterProblemHandlers(router, quietLogger{})
	// Only DELETE and PATCH are defined on this resource by TS 29.510, so a GET
	// of it must still produce a parseable error.
	router.DELETE("/nnrf-nfm/v1/subscriptions/:subscriptionID", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
	}{
		{"unserved path", http.MethodGet, "/no-such-resource", http.StatusNotFound},
		{"method the API does not define", http.MethodGet, "/nnrf-nfm/v1/subscriptions/abc", http.StatusMethodNotAllowed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), tc.method, tc.target, nil))

			if recorder.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
			if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, util.ContentTypeProblemJSON) {
				t.Errorf("Content-Type = %q, want %q", got, util.ContentTypeProblemJSON)
			}
			var problem map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
				t.Fatalf("body is not JSON (%v): %q", err, recorder.Body.String())
			}
			if _, hasTitle := problem["title"]; !hasTitle {
				t.Errorf("body is not a ProblemDetails: %q", recorder.Body.String())
			}
		})
	}
}
