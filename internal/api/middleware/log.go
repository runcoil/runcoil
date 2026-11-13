package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type responseRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *responseRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}

	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}

	n, err := r.ResponseWriter.Write(b)
	r.bytes += n

	return n, err
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// LogRequest logs one line per request: at info, or at error with the recorded error
// when the status is 500 or more.
func LogRequest(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			info := &requestInfo{}
			rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			ctx := withRequestInfo(r.Context(), info)

			next.ServeHTTP(rec, r.WithContext(ctx))

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", time.Since(start)),
				slog.Int("bytes", rec.bytes),
				slog.String("remote_addr", r.RemoteAddr),
			}

			if info.userID != "" {
				attrs = append(attrs, slog.String("user_id", info.userID))
			}

			if info.err != nil {
				attrs = append(attrs, slog.Any("error", info.err))
			}

			if info.stack != "" {
				attrs = append(attrs, slog.String("stack", info.stack))
			}

			level := slog.LevelInfo
			if rec.status >= http.StatusInternalServerError {
				level = slog.LevelError
			}

			logger.LogAttrs(ctx, level, "request", attrs...)
		})
	}
}
