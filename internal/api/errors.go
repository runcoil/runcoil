package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/apierr"
)

const internalMessage = "internal error"

var errorCodes = map[apierr.Code]gen.ErrorCode{
	apierr.CodeInvalidRequest:   gen.ErrorCodeInvalidRequest,
	apierr.CodeInvalidField:     gen.ErrorCodeInvalidField,
	apierr.CodeUnauthenticated:  gen.ErrorCodeUnauthenticated,
	apierr.CodePermissionDenied: gen.ErrorCodePermissionDenied,
}

func writeError(w http.ResponseWriter, status int, code gen.ErrorCode, message, field string) {
	apiErr := gen.Error{Code: code, Message: message}
	if field != "" {
		apiErr.Field = &field
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(gen.ErrorResponse{Errors: []gen.Error{apiErr}})
}

func (s *Service) handleError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		s.logger.ErrorContext(r.Context(), "request failed",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Any("error", err),
		)
		writeError(w, http.StatusInternalServerError, gen.ErrorCodeInternal, internalMessage, "")

		return
	}

	code, ok := errorCodes[apiErr.Code]
	if !ok {
		s.logger.ErrorContext(r.Context(), "api error has no dashboard code",
			slog.String("code", string(apiErr.Code)),
			slog.Any("error", err),
		)
		writeError(w, http.StatusInternalServerError, gen.ErrorCodeInternal, internalMessage, "")

		return
	}

	s.logger.WarnContext(r.Context(), "request refused",
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("code", string(code)),
		slog.String("message", apiErr.Message),
	)
	writeError(w, statusOf(apiErr.Kind), code, apiErr.Message, apiErr.Details["field"])
}

func (s *Service) handleRequestError(w http.ResponseWriter, _ *http.Request, err error) {
	writeError(w, http.StatusBadRequest, gen.ErrorCodeInvalidRequest, err.Error(), "")
}

func statusOf(kind apierr.Kind) int {
	switch kind {
	case apierr.KindInvalid:
		return http.StatusBadRequest
	case apierr.KindUnauthenticated:
		return http.StatusUnauthorized
	case apierr.KindPermissionDenied:
		return http.StatusForbidden
	case apierr.KindNotFound:
		return http.StatusNotFound
	case apierr.KindConflict, apierr.KindFailedPrecondition:
		return http.StatusConflict
	case apierr.KindUnavailable:
		return http.StatusServiceUnavailable
	case apierr.KindCanceled:
		return 499
	case apierr.KindDeadlineExceeded:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}
