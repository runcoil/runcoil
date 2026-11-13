package httperr

import (
	"encoding/json"
	"net/http"

	"github.com/runcoil/runcoil/internal/api/gen"
)

// Internal is the body of an unexpected failure.
var Internal = gen.ErrorResponse{Code: gen.ErrorCodeInternal, Message: "internal error"}

// New makes an error body. params name the request fields at fault.
func New(code gen.ErrorCode, message string, params ...gen.ErrorParam) gen.ErrorResponse {
	body := gen.ErrorResponse{Code: code, Message: message}
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
