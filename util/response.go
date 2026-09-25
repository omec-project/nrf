// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/omec-project/openapi/v2/utils"
)

const (
	// ContentTypeJSON is the media type of a successful response body.
	ContentTypeJSON = "application/json"
	// ContentTypeProblemJSON is the media type the 3GPP API definitions give
	// every error response, in place of application/json.
	ContentTypeProblemJSON = "application/problem+json"
)

// WarnLogger is the slice of a logger these handlers need, kept as a local
// interface so this package does not take a dependency on a logging library.
type WarnLogger interface {
	Warnf(template string, args ...any)
}

// ResponseContentType returns the media type a response with this status must
// carry: application/problem+json for an error, application/json otherwise.
func ResponseContentType(status int) string {
	if status >= http.StatusBadRequest {
		return ContentTypeProblemJSON
	}
	return ContentTypeJSON
}

// RegisterProblemHandlers makes gin answer an unserved path or an undefined
// method with a ProblemDetails, in place of its plain-text defaults, which an
// SBI client cannot parse.
func RegisterProblemHandlers(router *gin.Engine, log WarnLogger) {
	// gin leaves this false, which routes a request whose path exists but whose
	// method does not to NoRoute, answering "resource not found" for a resource
	// that does exist. Turning it on lets the method case be reported as such.
	router.HandleMethodNotAllowed = true

	router.NoRoute(func(c *gin.Context) {
		log.Warnf("no route for %s %s", c.Request.Method, c.Request.URL.Path)
		WriteProblem(c, http.StatusNotFound,
			utils.ProblemDetailsContextNotFound("resource not found"))
	})
	router.NoMethod(func(c *gin.Context) {
		log.Warnf("method %s not defined for %s", c.Request.Method, c.Request.URL.Path)
		WriteProblem(c, http.StatusMethodNotAllowed,
			utils.ProblemDetails("Method Not Allowed", http.StatusMethodNotAllowed,
				"the API does not define this method for this resource"))
	})
}

// WriteProblem serialises a ProblemDetails with the media type the API
// definitions give it. It exists because gin's c.JSON hard-codes
// application/json, which every error path in the NRF used to inherit.
func WriteProblem(c *gin.Context, status int, problem any) {
	c.Render(status, problemRender{status: status, data: problem})
}

type problemRender struct {
	status int
	data   any
}

func (r problemRender) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	encoded, err := json.Marshal(r.data)
	if err != nil {
		return err
	}
	_, err = w.Write(encoded)
	return err
}

func (r problemRender) WriteContentType(w http.ResponseWriter) {
	w.Header().Set("Content-Type", ContentTypeProblemJSON)
}
