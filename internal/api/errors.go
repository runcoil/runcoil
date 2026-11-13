package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/api/httperr"
	"github.com/runcoil/runcoil/internal/apierr"
)

func (s *Service) handleRequestError(w http.ResponseWriter, _ *http.Request, err error) {
	httperr.Write(w, http.StatusBadRequest, httperr.New(gen.ErrorCodeInvalidRequest, err.Error()))
}

func (s *Service) handleResponseError(w http.ResponseWriter, r *http.Request, err error) {
	logger := s.logger.With(
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
	)

	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		logger.ErrorContext(r.Context(), "request failed", slog.Any("error", err))
		httperr.Write(w, http.StatusInternalServerError, httperr.Internal)

		return
	}

	code := gen.ErrorCode(strings.ToLower(string(apiErr.Code)))
	if !code.Valid() {
		logger.ErrorContext(r.Context(), "api error code is not in the spec",
			slog.String("code", string(apiErr.Code)),
			slog.Any("error", err),
		)
		httperr.Write(w, http.StatusInternalServerError, httperr.Internal)

		return
	}

	logger.WarnContext(r.Context(), "request refused",
		slog.String("code", string(code)),
		slog.String("message", apiErr.Message),
	)

	var params []gen.ErrorParam
	if field := apiErr.Details["field"]; field != "" {
		params = append(params, gen.ErrorParam{Field: field, Message: apiErr.Message})
	}

	httperr.Write(w, statusOf(apiErr.Kind), httperr.New(code, apiErr.Message, params...))
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
