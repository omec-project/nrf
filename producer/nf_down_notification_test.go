// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/omec-project/openapi/v2/models"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// trackingReadCloser records whether it was read to EOF and closed, so tests
// can assert sendNFDownNotification drains the response body before closing
// it (required for the underlying transport to reuse the connection).
type trackingReadCloser struct {
	r         *strings.Reader
	readToEOF bool
	closed    bool
	totalRead int
}

func (t *trackingReadCloser) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	t.totalRead += n
	if err == io.EOF || t.r.Len() == 0 {
		t.readToEOF = true
	}
	return n, err
}

func (t *trackingReadCloser) Close() error {
	t.closed = true
	return nil
}

func withMockNfStatusNotifyTransport(t *testing.T, body *trackingReadCloser, statusCode int) {
	t.Helper()
	original := nfStatusNotifyHTTPClient
	nfStatusNotifyHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: statusCode,
				Body:       body,
				Header:     make(http.Header),
			}, nil
		}),
	}
	t.Cleanup(func() { nfStatusNotifyHTTPClient = original })
}

func TestSendNFDownNotificationDrainsAndClosesBody(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
	}{
		{name: "success status drains and closes body", statusCode: http.StatusNoContent},
		{name: "error status drains and closes body", statusCode: http.StatusInternalServerError},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackingReadCloser{r: strings.NewReader("amf down notification response")}
			withMockNfStatusNotifyTransport(t, body, tc.statusCode)

			nf := models.NewNFProfileWithDefaults()
			nf.SetNfType(models.NFTYPE_AMF)
			sendNFDownNotification(*nf, "test-amf-instance")

			if !body.readToEOF {
				t.Error("expected response body to be read to EOF")
			}
			if !body.closed {
				t.Error("expected response body to be closed")
			}
		})
	}
}

func TestSendNFStatusNotifyDrainsAndClosesBody(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		body       string
	}{
		{name: "success status drains and closes body", statusCode: http.StatusNoContent, body: ""},
		{name: "error status drains and closes body", statusCode: http.StatusBadRequest, body: `{"title":"bad request"}`},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackingReadCloser{r: strings.NewReader(tc.body)}
			withMockNfStatusNotifyTransport(t, body, tc.statusCode)

			SendNFStatusNotify(models.NOTIFICATIONEVENTTYPE_NF_REGISTERED, "http://nrf:29510/nnrf-nfm/v1/nf-instances/instance-1", "http://subscriber/callback")

			if !body.readToEOF {
				t.Error("expected response body to be read to EOF")
			}
			if !body.closed {
				t.Error("expected response body to be closed")
			}
		})
	}
}

func TestSendNFDownNotificationBoundsResponseBodyDrain(t *testing.T) {
	oversizedBody := strings.Repeat("a", 3*maxNotifyResponseBodyBytes)
	body := &trackingReadCloser{r: strings.NewReader(oversizedBody)}
	withMockNfStatusNotifyTransport(t, body, http.StatusNoContent)

	nf := models.NewNFProfileWithDefaults()
	nf.SetNfType(models.NFTYPE_AMF)
	sendNFDownNotification(*nf, "test-amf-instance")

	if body.readToEOF {
		t.Error("expected drain to stop at the byte cap, not read the oversized body to EOF")
	}
	if !body.closed {
		t.Error("expected response body to be closed even when the drain is capped")
	}
}

func TestSendNFStatusNotifyBoundsResponseBodyDrain(t *testing.T) {
	// The error-body read and deferred drain share one limiter, so a body
	// larger than the shared cap must remain unread after the call.
	oversizedBody := strings.Repeat("a", 3*maxNotifyResponseBodyBytes)
	body := &trackingReadCloser{r: strings.NewReader(oversizedBody)}
	withMockNfStatusNotifyTransport(t, body, http.StatusBadRequest)

	SendNFStatusNotify(models.NOTIFICATIONEVENTTYPE_NF_REGISTERED, "http://nrf:29510/nnrf-nfm/v1/nf-instances/instance-1", "http://subscriber/callback")

	if body.readToEOF {
		t.Error("expected drain to stop at the byte cap, not read the oversized body to EOF")
	}
	if !body.closed {
		t.Error("expected response body to be closed even when the drain is capped")
	}
}

func TestSendNFStatusNotifyErrorPathSharesTotalByteCap(t *testing.T) {
	// Oversized so that if the error-body read and the drain each applied
	// their own independent cap (the bug being regression-tested), their
	// combined read would exceed maxNotifyResponseBodyBytes.
	oversizedBody := strings.Repeat("a", 2*maxNotifyResponseBodyBytes)
	body := &trackingReadCloser{r: strings.NewReader(oversizedBody)}
	withMockNfStatusNotifyTransport(t, body, http.StatusBadRequest)

	SendNFStatusNotify(models.NOTIFICATIONEVENTTYPE_NF_REGISTERED, "http://nrf:29510/nnrf-nfm/v1/nf-instances/instance-1", "http://subscriber/callback")

	if body.totalRead > maxNotifyResponseBodyBytes {
		t.Errorf("expected error-body read and drain to share a single %d-byte cap, read %d bytes total", maxNotifyResponseBodyBytes, body.totalRead)
	}
}

func TestSendNFDownNotificationSkipsNonAmfNfType(t *testing.T) {
	called := false
	original := nfStatusNotifyHTTPClient
	nfStatusNotifyHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			called = true
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Header: make(http.Header)}, nil
		}),
	}
	t.Cleanup(func() { nfStatusNotifyHTTPClient = original })

	nf := models.NewNFProfileWithDefaults()
	nf.SetNfType(models.NFTYPE_AUSF)
	sendNFDownNotification(*nf, "test-ausf-instance")

	if called {
		t.Error("expected no notification request for a non-AMF NF type")
	}
}
