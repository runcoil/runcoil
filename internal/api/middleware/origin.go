package middleware

import (
	"net/http"

	"github.com/runcoil/runcoil/internal/api/httperr"
	"github.com/runcoil/runcoil/internal/apperr"
)

// CheckOrigin refuses a write that a browser sends from another origin.
func CheckOrigin() func(http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httperr.Write(w, http.StatusForbidden, httperr.New(apperr.CodePermissionDenied, "cross-origin request refused"))
	}))

	return protection.Handler
}
