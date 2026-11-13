package httperr

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/apierr"
)

// Internal is the body of an unexpected failure.
var Internal = New(apierr.CodeInternal, "internal error")

// New makes an error body with code in lower case. params name the request fields at
// fault.
func New(code apierr.Code, message string, params ...gen.ErrorParam) gen.ErrorResponse {
	body := gen.ErrorResponse{Code: strings.ToLower(string(code)), Message: message}
	if len(params) > 0 {
		body.Params = &params
	}

	return body
}

// Write writes an error body with status.
func Write(w http.ResponseWriter, status int, body gen.ErrorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
