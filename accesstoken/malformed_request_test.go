// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package accesstoken_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/omec-project/nrf/accesstoken"
)

// A request body the NRF cannot bind is answered with a ProblemDetails served
// as application/problem+json, like every other error on the interface. The
// bind must not commit a response of its own first: headers written before the
// handler's error branch runs cannot be changed afterwards, so the client would
// receive the ProblemDetails body under whatever media type the bind left.
func TestMalformedAccessTokenRequestIsAProblemDetails(t *testing.T) {
	router := accesstoken.NewRouter()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/oauth2/token", strings.NewReader("{not json"))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	// Result, not recorder.Header: the recorder's live header map still shows
	// changes made after the status line was written, which a real server has
	// already sent without them. Result carries what the client received.
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
	if got := response.Header.Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/problem+json")
	}
}
