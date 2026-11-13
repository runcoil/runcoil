package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/runcoil/runcoil/internal/api/httperr"
)

// Recover answers a panicking request with an internal error, unless the response has
// started, and puts the panic and its stack on the request's log line.
func Recover() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

			defer func() {
				v := recover()
				if v == nil {
					return
				}

				if v == http.ErrAbortHandler { //nolint:errorlint // recover returns the value as it was panicked.
					panic(v)
				}

				recordPanic(r.Context(), fmt.Errorf("panic: %v", v), string(debug.Stack()))

				if !rec.wroteHeader {
					httperr.Write(rec, http.StatusInternalServerError, httperr.Internal)
				}
			}()

			next.ServeHTTP(rec, r)
		})
	}
}
