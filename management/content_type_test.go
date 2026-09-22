// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package management

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// requireContentType guards the request bodies. The accepted media type differs
// per operation: TS 29.510 gives the PATCH operations
// application/json-patch+json, which is what every NF's heartbeat sends, so a
// check that accepted only application/json would reject the whole core.
func TestRequireContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		accepted    []string
		wantAllowed bool
	}{
		{"exact match", contentTypeJSON, []string{contentTypeJSON}, true},
		{"parameters are ignored", "application/json; charset=utf-8", []string{contentTypeJSON}, true},
		{"case is ignored", "APPLICATION/JSON", []string{contentTypeJSON}, true},
		{"unsupported type", "text/plain", []string{contentTypeJSON}, false},
		{"json is not json-patch", contentTypeJSON, []string{contentTypeJSONPatch}, false},
		{"json-patch accepted where defined", contentTypeJSONPatch, []string{contentTypeJSONPatch}, true},
		{"unparseable header", "not a media type", []string{contentTypeJSON}, false},
		// The NRF has never required the header; rejecting its absence would
		// change behaviour for clients we have no evidence about.
		{"absent header is tolerated", "", []string{contentTypeJSON}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequestWithContext(context.Background(), http.MethodPut,
				"/nnrf-nfm/v1/nf-instances/x", strings.NewReader("{}"))
			if tc.contentType != "" {
				c.Request.Header.Set("Content-Type", tc.contentType)
			} else {
				c.Request.Header.Del("Content-Type")
			}

			allowed := requireContentType(c, tc.accepted...)

			if allowed != tc.wantAllowed {
				t.Fatalf("allowed = %v, want %v", allowed, tc.wantAllowed)
			}
			if !tc.wantAllowed {
				if recorder.Code != http.StatusUnsupportedMediaType {
					t.Errorf("status = %d, want %d", recorder.Code, http.StatusUnsupportedMediaType)
				}
				if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, contentTypeProblemJSON) {
					t.Errorf("Content-Type = %q, want %q", got, contentTypeProblemJSON)
				}
			}
		})
	}
}
