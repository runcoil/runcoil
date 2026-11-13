package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/runcoil/runcoil/internal/api/httperr"
)

// Recover answers a panicking request with an internal error and logs the panic.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}

				if v == http.ErrAbortHandler { //nolint:errorlint // recover returns the value as it was panicked.
					panic(v)
				}

				logger.ErrorContext(r.Context(), "request panicked",
					slog.Any("panic", v),
					slog.String("stack", string(debug.Stack())),
				)
				httperr.Write(w, http.StatusInternalServerError, httperr.Internal)
			}()

			next.ServeHTTP(w, r)
		})
	}
}
